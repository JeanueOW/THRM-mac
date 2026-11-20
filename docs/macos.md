# 在 macOS 上构建与使用 THRM

THRM 的 macOS 版与 Windows/Linux 版共用同一套 Go 核心、Wails GUI 和 Next.js 前端。
设备协议、温控算法、曲线方案、SmartControl、托盘与快捷键逻辑完全复用，差异集中在
系统集成层：

| 能力 | macOS 实现 |
| --- | --- |
| 设备通信 | `go-hid`（IOKit HID）/ `tinygo.org/x/bluetooth`（CoreBluetooth） |
| CPU / GPU 温度 | Intel：AppleSMC；Apple Silicon：IOHIDEventSystemClient PMU 传感器 |
| CPU 功耗 | AppleSMC 电源键（`PCTR`/`PCPT`/`PCPC`） |
| GPU 功耗 / 型号 | `IOAccelerator` 的 `PerformanceStatistics` 与 PCI `model` 属性 |
| 后台服务 | 独立 `thrm-core` 进程，GUI 关闭后继续控温 |
| 菜单栏 | `fyne.io/systray` + AppKit 主线程事件循环 |
| 自启动 | `~/Library/LaunchAgents/top.tianli0.thrm.core.plist` |
| 全局快捷键 | Carbon `RegisterEventHotKey`（默认 `Ctrl+Alt+Shift+M/A/C`） |
| 睡眠/唤醒 | IOKit `IORegisterForSystemPower` |
| 配置 | `~/Library/Application Support/THRM/config.json` |
| 日志 / 崩溃报告 | `~/Library/Logs/THRM/` |
| 温度历史 | `~/Library/Application Support/THRM/telemetry/history.bin` |

## 系统要求

- macOS 13 Ventura 或更高版本
- Intel 或 Apple Silicon
- 构建需要 Xcode Command Line Tools（`xcode-select --install`）
- Go 1.27.3+、Bun、Wails v3 CLI

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
curl -fsSL https://bun.sh/install | bash
```

## 构建

```bash
./build_macos.sh
```

产物：

```text
build/bin/THRM.app
build/bin/THRM-<version>-macOS-amd64.dmg   # 或 arm64 / universal
```

脚本会：

1. 构建 GUI（`wails3 task build`，内含前端 `bun run build` 与 bindings 生成）；
2. 构建核心服务 `thrm-core`；
3. 组装 `THRM.app/Contents/MacOS/{THRM,thrm-core}`；
4. 复制 `Info.plist` 与 `icons.icns`；
5. 用 `codesign` 签名（默认 ad-hoc）；
6. 生成 DMG。

通用二进制：

```bash
UNIVERSAL=1 ./build_macos.sh
```

> 交叉编译 arm64 需要 Clang 能同时处理两种架构，脚本已设置好 `-arch` 参数。
> 在 Apple Silicon 上本机构建 arm64 更简单。

安装：把 `THRM.app` 拖进「应用程序」后启动。首次启动会写入配置并启动核心服务。

本地 ad-hoc 构建如果要拷贝到其他 Mac，Gatekeeper 会拦截；接收方需要右键「打开」，
或者使用 Developer ID 签名并公证后再分发。

## 代码签名与权限（重要）

macOS 的 TCC 会对蓝牙、麦克风等隐私权限做代码签名检查。**从 macOS 26 开始，
ad-hoc 签名的应用会被直接拒绝弹出权限请求**（系统日志中为
`Policy disallows prompt ... access to kTCCServiceBluetoothAlways denied`）。
因此：

- **HID 设备（BS2 / BS2 Pro / BS3 / BS3 Pro）不受影响**：THRM 通过 IOKit 直接访问
  厂商 HID 接口，不需要蓝牙隐私权限。
- **BS1（BLE）和噪音校准（麦克风）需要正式签名**。

### 正式签名

使用 Apple Developer ID 证书：

```bash
CODESIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" ./build_macos.sh
```

脚本会使用 hardened runtime，并附带：

```text
com.apple.security.device.bluetooth
com.apple.security.device.audio-input
```

公证（需要提前用 `xcrun notarytool store-credentials thrm-notary ...` 保存凭据）：

```bash
NOTARIZE=1 NOTARY_PROFILE=thrm-notary \
CODESIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
./build_macos.sh
```

### 本地未签名构建

1. 打开「系统设置 → 隐私与安全性 → 蓝牙」，点列表下方的 **+**，手动添加
   `THRM.app`（若列表允许添加），然后重新启动 THRM。
2. 或在终端里先启动核心（终端本身需要蓝牙权限），再启动 `THRM.app`：

   ```bash
   ./build/bin/thrm-core --debug &
   open build/bin/THRM.app
   ```

   这种方式下核心以「终端」为责任进程，适合开发调试 BS1。

麦克风权限同理：未签名构建可能无法弹出授权框，需要在「隐私与安全性 → 麦克风」
中手动添加 `THRM.app`。

## 设备连接

### BS2 / BS2 Pro / BS3 / BS3 Pro（HID）

1. 先在「系统设置 → 蓝牙」里完成系统配对。
2. 启动 THRM，核心会自动发现厂商 HID 接口并连接。

### BS1（BLE）

THRM 直接通过 CoreBluetooth 扫描，不依赖系统蓝牙列表。BS1 只在刚上电/刚断开时
广播 `Flydigi BS1` 名称，其余时间只广播服务 UUID `0xFFF0`；核心的扫描匹配器会：

- 名字含 `BS1` 的广播立即采用；
- 无名称但带 `0xFFF0` 的广播先作为候选，3 秒内没有带名字的广播就按服务 UUID 采用；
- 记住上次连接的设备 UUID，后续优先直连。

macOS 没有 Windows 上「显示所有蓝牙设备」这类开关，CoreBluetooth 扫描本身就能看到
未命名外设。首次使用需要在权限弹窗或系统设置中允许 THRM 使用蓝牙。

## 温度与功耗

- **Intel Mac**：通过 AppleSMC 读取 `TC0D/TC0P/TG0D/TG0P` 等键，CPU/GPU 温度均可显示。
- **Apple Silicon**：通过 IOHIDEventSystemClient 读取 `PMU tdie*` 等传感器；GPU 温度
  取决于系统暴露的传感器名称（名称含 `GPU` 的会被识别）。
- **CPU 功耗**：从 AppleSMC 的 `PCTR` / `PCPT` / `PCPC` 等电源键读取（单位 W，
  兼容 VirtualSMC 的 `sp96` 等定点格式）。真实 Mac 与黑苹果都无需 root。
- **GPU 功耗与型号**：从 `IOAccelerator` 的 `PerformanceStatistics` 读取
  `Total Power(W)` / `GPU Power(W)` / `Instant Power(W)`，型号取自父级
  `IOPCIDevice` 的 `model` 属性。NVIDIA、AMD、Intel 驱动都会发布这份字典。
- 如果 Apple Silicon 机器没有暴露 CPU 电源键、驱动也未提供 PerformanceStatistics，
  对应功耗会显示为不可用（0），不影响温度控温与风扇曲线。
- GPU 功耗键名与 [SMCNvidiaGPU](https://github.com/zzzyy678/SMCNvidiaGPU) 及
  iStat Menus 使用的 `PerformanceStatistics` 约定一致；无需安装该 kext。
- 笔记本内置风扇转速（`laptopfan`）目前仅 Windows 实现，macOS 不显示该行。

## 自启动

在设置页打开「开机自启动」后，核心会写入 LaunchAgent：

```text
~/Library/LaunchAgents/top.tianli0.thrm.core.plist
```

它只启动 `thrm-core`，不弹出窗口；之后可从菜单栏图标打开界面。应用被移动到其他
位置后，核心下次启动会自动重写 LaunchAgent 中的路径。

macOS 26 会把新写入的 LaunchAgent 纳入「系统设置 → 通用 → 登录项与扩展」的后台
项目，首次开启时可能弹出“THRM Core 可在后台运行”的通知：请选择允许，否则开机
自启动会被系统拦下。

卸载时删除：

```bash
rm -f ~/Library/LaunchAgents/top.tianli0.thrm.core.plist
rm -rf "$HOME/Library/Application Support/THRM" "$HOME/Library/Logs/THRM"
```

## 已知限制

- 无 RTSS 游戏内叠加层（Windows 专用）。
- 无 PawnIO / LibreHardwareMonitor 桥接（Windows 专用），温度由原生传感器提供。
- 无亚克力/云母窗口材质（Windows 专用），窗口使用系统原生标题栏。
- 无 RGB 灯效之外的厂商联动（联想 Fn+Q、飞智空间站兼容等为 Windows 专用）。
- 固件刷写依赖 Windows 驱动，macOS 上不可用。
- BS1 的 BLE 权限与麦克风权限受代码签名影响，见上文。

## 开发

```bash
# 前端热更新 + Go 自动重编译
wails3 task dev

# 只构建后端
wails3 task build

# 运行（开发模式直接运行未打包二进制）
wails3 task run
```

`build/darwin/Info.plist`、`Info.dev.plist`、`icons.icns` 与 `entitlements.plist`
是 macOS 专用资产，已提交到仓库。不要用 `wails3 update build-assets -dir build`
覆盖它们（Taskfile 的 `assets` 任务写入 `build/.wails`，不会碰这些文件）。
