//go:build !darwin

package guiapp

// platformKeepsWindowAliveOnClose 在 Windows/Linux 上保持原有的关闭语义：
// 关闭窗口即关闭 GUI 进程，后台控温仍由核心服务的托盘接管。
func platformKeepsWindowAliveOnClose() bool { return false }
