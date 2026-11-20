//go:build !darwin

package coreapp

// platformForcesTrayVisible 在 Windows/Linux 上允许用户隐藏托盘图标。
func platformForcesTrayVisible() bool { return false }
