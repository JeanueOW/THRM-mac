# Build Directory

Build configuration and assets for the THRM GUI (Wails v3). Run the tasks from the repository root
with `wails3 task <name>` — see [`Taskfile.yml`](../Taskfile.yml).

* `config.yml` - Wails v3 project configuration. `info.version` is the single source of the THRM
  version; `build.bat`, `build.sh`, CI and `PKGBUILD` all read it. `dev_mode` drives `wails3 dev`.
* `appicon.png` - Application icon, also embedded as the Linux window icon.
* `bin` - Output directory.
* `.wails` - Assets rendered from `config.yml` during a build (ignored by git).

## Windows

* `icon.ico` - Executable and installer icon.
* `info.json` - Version resource template; `@VERSION@` is replaced with `info.version` at build time.
  It deliberately declares `FileVersion` and the fixed `product_version`, which Wails' own template
  omits (Windows then shows no version details at all).
* `wails.exe.manifest` - Application manifest. Maintained by hand because THRM requires
  administrator rights; it is not regenerated from `config.yml`.
* `installer/*` - The installer, built with [FluentSetup](https://github.com/TIANLI0/FluentSetup):
  `fluentsetup.json` (install folder, PawnIO, autostart, upgrades from NSIS installs, uninstall cleanup),
  `locales/*.json` (option texts and license per language) and `resources/license_*.txt`. `wails3 task package`
  runs `fluentsetup build fluentsetup.json -D Version=<config.yml version> -D PawnIOVersion=<pawnio.env> -o ../../bin`;
  FluentSetup downloads Microsoft's WebView2 bootstrapper itself (`"auto": "bundle"`).

## macOS

* `darwin/Info.plist`, `darwin/Info.dev.plist` - Bundle metadata. Besides the Wails defaults these
  declare `NSBluetoothAlwaysUsageDescription`, `NSMicrophoneUsageDescription`, the minimum macOS
  version (13.0) and the app category. They are maintained by hand; `wails3 task assets` writes to
  `.wails` and does not overwrite them.
* `darwin/icons.icns` - Application icon, generated from `appicon.png` with
  `wails3 generate icons -input build/appicon.png -macfilename build/darwin/icons.icns`.
* `darwin/dmg-background.png` - Finder window background for the DMG installer. `build_macos.sh`
  mounts a read-write image, places the icons and writes the Finder layout, then converts to UDZO.
* `darwin/entitlements.plist` - Hardened-runtime entitlements (`device.bluetooth`, `device.audio-input`)
  used when building with a Developer ID identity.

`build_macos.sh` assembles `build/bin/THRM.app` with both `THRM` and `thrm-core` in
`Contents/MacOS`, then ad-hoc or Developer ID signs it and creates a DMG. See
[`docs/macos.md`](../docs/macos.md).
