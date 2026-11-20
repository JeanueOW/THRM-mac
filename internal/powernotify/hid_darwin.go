//go:build darwin

package powernotify

import (
	"sync"
	"time"

	"github.com/sstallion/go-hid"
)

// RegisterHIDInterfaceArrivalNotifications polls IOKit for the configured HID
// interfaces. macOS has no public equivalent of WM_DEVICECHANGE for third-party
// apps that does not also require a privileged helper or a CFRunLoop of its
// own; polling at this interval costs a fraction of a millisecond and only
// produces a callback when a matching device path actually appears.
func RegisterHIDInterfaceArrivalNotifications(devices []HIDDeviceID, onArrival func(string)) (func(), error) {
	if len(devices) == 0 {
		return func() {}, nil
	}

	stop := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		known := make(map[string]struct{})
		first := true
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			current := make(map[string]struct{})
			for _, device := range devices {
				_ = hid.Enumerate(device.VendorID, device.ProductID, func(info *hid.DeviceInfo) error {
					if info != nil && info.Path != "" {
						current[info.Path] = struct{}{}
					}
					return nil
				})
			}

			if first {
				for path := range current {
					known[path] = struct{}{}
				}
				first = false
			} else {
				for path := range current {
					if _, exists := known[path]; !exists {
						known[path] = struct{}{}
						if onArrival != nil {
							onArrival(path)
						}
					}
				}
				for path := range known {
					if _, exists := current[path]; !exists {
						delete(known, path)
					}
				}
			}

			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()

	return func() {
		stopOnce.Do(func() { close(stop) })
	}, nil
}
