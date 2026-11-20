#!/bin/bash
# THRM macOS build script.
#
#   ./build_macos.sh                # host architecture, ad-hoc signed .app + dmg
#   UNIVERSAL=1 ./build_macos.sh    # Intel + Apple Silicon universal app
#   CODESIGN_IDENTITY="Developer ID Application: ..." ./build_macos.sh
#
# Outputs:
#   build/bin/THRM.app
#   build/bin/THRM-<version>-macOS-<arch>.dmg (or .zip when hdiutil is unavailable)
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$PROJECT_ROOT/build"
BIN_DIR="$BUILD_DIR/bin"
APP_DIR="$BIN_DIR/THRM.app"
export PATH="$HOME/go/bin:$HOME/.bun/bin:$PATH"

echo "=== THRM macOS Build ==="

VERSION="${THRM_BUILD_VERSION:-}"
if [ -z "$VERSION" ]; then
    VERSION=$(sed -nE 's/^[[:space:]]+version:[[:space:]]*"?([^"#[:space:]]+)"?.*/\1/p' "$PROJECT_ROOT/build/config.yml" | head -n1)
fi
if [ -z "$VERSION" ]; then
    echo "WARNING: could not resolve version from build/config.yml, using 'dev'"
    VERSION="dev"
fi
echo "Building version: $VERSION"

LDFLAGS="-s -w -X github.com/TIANLI0/THRM/internal/version.BuildVersion=$VERSION"
export CGO_ENABLED=1
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
export MACOSX_DEPLOYMENT_TARGET=13.0

mkdir -p "$BIN_DIR"

build_one() {
    local goarch="$1"
    local suffix="$2"
    local cgo_arch
    case "$goarch" in
        arm64) cgo_arch="arm64" ;;
        *)     cgo_arch="x86_64" ;;
    esac
    export CGO_CFLAGS="-arch $cgo_arch -mmacosx-version-min=13.0"
    export CGO_LDFLAGS="-arch $cgo_arch -mmacosx-version-min=13.0"
    export MACOSX_DEPLOYMENT_TARGET=13.0

    echo "--- Building GUI ($goarch) ---"
    GOARCH="$goarch" wails3 task build VERSION="$VERSION" BIN_DIR="$BIN_DIR" >/dev/null
    mv -f "$BIN_DIR/THRM" "$BIN_DIR/THRM$suffix"

    echo "--- Building core service ($goarch) ---"
    GOARCH="$goarch" go build \
        -trimpath \
        -ldflags="$LDFLAGS" \
        -o "$BIN_DIR/thrm-core$suffix" \
        ./cmd/core/
}

# Remove the previous build outputs. A leftover universal (fat) binary cannot be
# overwritten by `go build -o` on some Go versions; the linker reports it as
# "already exists and is not an object file".
rm -f "$BIN_DIR/THRM" "$BIN_DIR/thrm-core" \
    "$BIN_DIR/THRM-amd64" "$BIN_DIR/THRM-arm64" \
    "$BIN_DIR/thrm-core-amd64" "$BIN_DIR/thrm-core-arm64"
if [ "${UNIVERSAL:-0}" = "1" ]; then
    build_one amd64 -amd64
    build_one arm64 -arm64
    echo "--- Creating universal binaries ---"
    lipo -create -output "$BIN_DIR/THRM" "$BIN_DIR/THRM-amd64" "$BIN_DIR/THRM-arm64"
    lipo -create -output "$BIN_DIR/thrm-core" "$BIN_DIR/thrm-core-amd64" "$BIN_DIR/thrm-core-arm64"
    rm -f "$BIN_DIR/THRM-amd64" "$BIN_DIR/THRM-arm64" "$BIN_DIR/thrm-core-amd64" "$BIN_DIR/thrm-core-arm64"
    ARCH_LABEL="universal"
else
    build_one "$(go env GOARCH)" ""
    ARCH_LABEL="$(go env GOARCH)"
fi

echo "--- Assembling $APP_DIR ---"
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"

cp "$BIN_DIR/THRM" "$APP_DIR/Contents/MacOS/THRM"
cp "$BIN_DIR/thrm-core" "$APP_DIR/Contents/MacOS/thrm-core"
chmod +x "$APP_DIR/Contents/MacOS/THRM" "$APP_DIR/Contents/MacOS/thrm-core"
cp "$BUILD_DIR/darwin/Info.plist" "$APP_DIR/Contents/Info.plist"
cp "$BUILD_DIR/darwin/icons.icns" "$APP_DIR/Contents/Resources/icons.icns"
printf 'APPL????' > "$APP_DIR/Contents/PkgInfo"

# Keep the bundle version in sync with build/config.yml. CFBundleVersion must be
# numeric, so strip suffixes such as "-test.1" from the display version.
BUNDLE_VERSION=$(printf '%s' "$VERSION" | sed -nE 's/^[^0-9]*([0-9]+(\.[0-9]+)*).*/\1/p')
if [ -z "$BUNDLE_VERSION" ]; then
    BUNDLE_VERSION="0.0.0"
fi
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $BUNDLE_VERSION" "$APP_DIR/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$APP_DIR/Contents/Info.plist"

# The GUI locates thrm-core next to itself inside Contents/MacOS. Keeping both
# binaries in the bundle is also what makes the Bluetooth usage description in
# Info.plist available to CoreBluetooth when the core is started by launchd.

SIGN_IDENTITY="${CODESIGN_IDENTITY:--}"
ENTITLEMENTS="$BUILD_DIR/darwin/entitlements.plist"
echo "--- Signing with: $SIGN_IDENTITY ---"
xattr -cr "$APP_DIR" || true
if [ "$SIGN_IDENTITY" = "-" ]; then
    # Ad-hoc signing for local use. macOS 26 refuses to show Bluetooth and
    # microphone permission prompts for ad-hoc apps; use a Developer ID build
    # for those features (see docs/macos.md).
    codesign --force --deep --sign - "$APP_DIR"
else
    # Sign nested code first, then the bundle. Hardened runtime + the
    # Bluetooth/audio-input entitlements are required for TCC to prompt.
    codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS" --sign "$SIGN_IDENTITY" "$APP_DIR/Contents/MacOS/thrm-core"
    codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS" --sign "$SIGN_IDENTITY" "$APP_DIR/Contents/MacOS/THRM"
    codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS" --sign "$SIGN_IDENTITY" "$APP_DIR"
fi
codesign --verify --deep --strict "$APP_DIR"
echo "Signature OK"

if [ "${NOTARIZE:-0}" = "1" ]; then
    PROFILE="${NOTARY_PROFILE:-thrm-notary}"
    echo "--- Notarizing with keychain profile: $PROFILE ---"
    ditto -c -k --keepParent "$APP_DIR" "$BIN_DIR/THRM-notarize.zip"
    xcrun notarytool submit "$BIN_DIR/THRM-notarize.zip" --keychain-profile "$PROFILE" --wait
    xcrun stapler staple "$APP_DIR"
    rm -f "$BIN_DIR/THRM-notarize.zip"
    echo "Notarization OK"
fi

echo "--- Creating archive ---"
OUT_BASE="$BIN_DIR/THRM-${VERSION}-macOS-${ARCH_LABEL}"
rm -f "$OUT_BASE.dmg" "$OUT_BASE.zip"
if command -v hdiutil >/dev/null 2>&1; then
    hdiutil create -volname "THRM $VERSION" -srcfolder "$APP_DIR" -ov -format UDZO "$OUT_BASE.dmg" >/dev/null
    echo "DMG: $OUT_BASE.dmg"
else
    (cd "$BIN_DIR" && ditto -c -k --sequesterRsrc --keepParent "THRM.app" "$OUT_BASE.zip")
    echo "ZIP: $OUT_BASE.zip"
fi

echo "=== Build complete ==="
echo "App: $APP_DIR"
echo "GUI: $BIN_DIR/THRM"
echo "Core: $BIN_DIR/thrm-core"
