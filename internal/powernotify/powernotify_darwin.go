//go:build darwin

package powernotify

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
int thrmRegisterPowerNotifications(void);
void thrmUnregisterPowerNotifications(void);
*/
import "C"

import (
	"fmt"
	"sync"
)

var (
	darwinSuspendCallback func()
	darwinResumeCallback  func(userPresent bool)
	darwinCallbackMutex   sync.RWMutex
	darwinStopOnce        sync.Once
)

// RegisterSuspendResumeNotifications subscribes to IOKit system power
// notifications. The notification run-loop source is attached to the main
// run loop, which the core process drives through the menu bar item.
func RegisterSuspendResumeNotifications(onSuspend func(), onResume func(userPresent bool)) (func(), error) {
	darwinCallbackMutex.Lock()
	darwinSuspendCallback = onSuspend
	darwinResumeCallback = onResume
	darwinCallbackMutex.Unlock()

	darwinStopOnce = sync.Once{}
	if status := C.thrmRegisterPowerNotifications(); status != 0 {
		return nil, fmt.Errorf("IORegisterForSystemPower failed: %d", int(status))
	}

	return func() {
		darwinStopOnce.Do(func() {
			C.thrmUnregisterPowerNotifications()
			darwinCallbackMutex.Lock()
			darwinSuspendCallback = nil
			darwinResumeCallback = nil
			darwinCallbackMutex.Unlock()
		})
	}, nil
}

//export thrmPowerEvent
func thrmPowerEvent(kind C.int) {
	darwinCallbackMutex.RLock()
	suspend := darwinSuspendCallback
	resume := darwinResumeCallback
	darwinCallbackMutex.RUnlock()

	switch int(kind) {
	case 0:
		if suspend != nil {
			suspend()
		}
	case 1:
		if resume != nil {
			// IOKit does not tell us whether this wake was user-initiated.
			// Resuming fan control is safe either way; the cooler simply goes
			// back to the configured curve.
			resume(true)
		}
	}
}
