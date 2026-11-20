//go:build darwin

package coreapp

// platformForcesTrayVisible 报告本平台是否必须保留菜单栏图标。
//
// macOS 上的核心进程是后台代理（activation policy accessory），没有 Dock
// 图标。关掉菜单栏图标后，用户只能通过重新启动 GUI 来找回窗口，因此配置里的
// “隐藏托盘”在 macOS 上不生效。
func platformForcesTrayVisible() bool { return true }
