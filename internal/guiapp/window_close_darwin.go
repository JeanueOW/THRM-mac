//go:build darwin

package guiapp

// platformKeepsWindowAliveOnClose 报告关闭主窗口时是否只隐藏窗口。
//
// macOS 用户习惯用红色关闭按钮收起窗口，而应用（和菜单栏图标）继续运行。
// 隐藏而不是销毁窗口，托盘里的“显示窗口”和点击 Dock 图标才能立即恢复原界面，
// 不必重建 WebView 并丢失标签页状态。
func platformKeepsWindowAliveOnClose() bool { return true }
