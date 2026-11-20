//go:build !darwin

package tray

func platformRunsTrayOnMainThread() bool { return false }

// RunMainThread is a no-op on platforms where systray owns its own message
// loop thread.
func (m *Manager) RunMainThread() {}
