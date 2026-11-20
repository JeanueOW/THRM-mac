//go:build darwin

package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void thrmSetAccessoryActivationPolicy(void) {
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
*/
import "C"

// platformRunsTrayOnMainThread reports that AppKit requires the status item
// and its NSApplication run loop to live on the process main thread.
func platformRunsTrayOnMainThread() bool { return true }

// RunMainThread blocks in the NSApplication event loop and must be called from
// the process main goroutine. The core process has no other UI, so this is the
// main loop for the whole daemon on macOS.
func (m *Manager) RunMainThread() {
	// Accessory policy keeps the background core out of the Dock and Cmd-Tab;
	// the status item is the only UI it owns. The GUI is a separate app bundle.
	C.thrmSetAccessoryActivationPolicy()
	m.runSystrayInstance()
}
