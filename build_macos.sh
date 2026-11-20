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
    # A Finder drag-to-Applications target is what users expect from a macOS
    # installer. Stage the bundle with an /Applications symlink and create a
    # read-write image so Finder can store icon positions and a background.
    VOLUME_NAME="THRM $VERSION"
    DMG_STAGE="$(mktemp -d "${TMPDIR:-/tmp}/thrm-dmg.XXXXXX")"
    DMG_MOUNT=""
    RW_DMG="$(mktemp -u "${TMPDIR:-/tmp}/thrm-rw.XXXXXX.dmg")"
    trap 'if [ -n "${DMG_MOUNT:-}" ]; then hdiutil detach "$DMG_MOUNT" -force >/dev/null 2>&1 || true; fi; rm -rf "$DMG_STAGE" "$RW_DMG"' EXIT

    ditto "$APP_DIR" "$DMG_STAGE/THRM.app"
    ln -s /Applications "$DMG_STAGE/Applications"

    # hdiutil appends " 1" when a volume with the same name is already mounted,
    # and Finder would then lay out the stale volume instead of the new image.
    if [ -d "/Volumes/$VOLUME_NAME" ]; then
        echo "Detaching previously mounted /Volumes/$VOLUME_NAME"
        hdiutil detach "/Volumes/$VOLUME_NAME" >/dev/null 2>&1 || true
    fi

    hdiutil create -volname "$VOLUME_NAME" -srcfolder "$DMG_STAGE" -fs HFS+ -format UDRW -ov "$RW_DMG" >/dev/null
    # Mount at the default /Volumes/<volume name> location: Finder takes the
    # disk name from the mount point, and a custom -mountpoint would rename it
    # to the temporary directory name.
    ATTACH_OUTPUT="$(hdiutil attach -noautoopen -readwrite "$RW_DMG" 2>/dev/null || true)"
    DMG_MOUNT="$(printf '%s\n' "$ATTACH_OUTPUT" | awk -F'\t' '/Apple_HFS/ {print $NF}' | tail -n1)"
    if [ -n "$DMG_MOUNT" ] && [ -d "$DMG_MOUNT" ]; then
        mkdir -p "$DMG_MOUNT/.background"
        cp "$BUILD_DIR/darwin/dmg-background.png" "$DMG_MOUNT/.background/background.png"

        if osascript <<APPLESCRIPT
tell application "Finder"
    tell disk "$VOLUME_NAME"
        open
        set current view of container window to icon view
        set toolbar visible of container window to false
        set statusbar visible of container window to false
        set the bounds of container window to {240, 180, 880, 608}
        set viewOptions to the icon view options of container window
        set arrangement of viewOptions to not arranged
        set icon size of viewOptions to 96
        set text size of viewOptions to 13
        set background picture of viewOptions to file ".background:background.png"
        set position of item "THRM.app" of container window to {170, 200}
        set position of item "Applications" of container window to {470, 200}
        update without registering applications
        delay 1
        close
    end tell
end tell
APPLESCRIPT
        then
            [ -f "$DMG_MOUNT/.DS_Store" ] && echo "DMG layout written"
        else
            echo "WARNING: Finder layout script failed; falling back to a plain DMG (Applications shortcut still present)"
        fi

        sync
        hdiutil detach "$DMG_MOUNT" >/dev/null 2>&1 || hdiutil detach "$DMG_MOUNT" -force >/dev/null 2>&1 || true
        DMG_MOUNT=""
    else
        echo "WARNING: could not mount the temporary DMG read-write; creating a plain DMG"
    fi

    rm -f "$OUT_BASE.dmg"
    if [ -f "$RW_DMG" ]; then
        hdiutil convert "$RW_DMG" -format UDZO -o "$OUT_BASE.dmg" >/dev/null
    else
        hdiutil create -volname "$VOLUME_NAME" -srcfolder "$DMG_STAGE" -ov -format UDZO "$OUT_BASE.dmg" >/dev/null
    fi
    rm -rf "$DMG_STAGE" "$RW_DMG"
    echo "DMG: $OUT_BASE.dmg"
else
    (cd "$BIN_DIR" && ditto -c -k --sequesterRsrc --keepParent "THRM.app" "$OUT_BASE.zip")
    echo "ZIP: $OUT_BASE.zip"
fi

echo "=== Build complete ==="
echo "App: $APP_DIR"
echo "GUI: $BIN_DIR/THRM"
echo "Core: $BIN_DIR/thrm-core"
