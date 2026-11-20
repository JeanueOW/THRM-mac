//go:build darwin

package device

import (
	"strings"
	"time"

	"github.com/TIANLI0/THRM/internal/deviceproto"
)

// macOS hidapi issues a synchronous IOHIDDeviceSetReport. For Bluetooth HID,
// the firmware briefly refuses new output reports while its flash routine is
// erasing/programming a page, and SetReport then fails with kIOReturnError
// (0xE00002BC). Windows queues the report instead and does not surface this.
// Retrying the idempotent flash-write commands after a short pause avoids
// spurious "apply light strip failed" errors on reconnect.
func hidWriteRetryDelays(report []byte, err error) []time.Duration {
	if err == nil {
		return nil
	}
	message := err.Error()
	if !strings.Contains(message, "IOHIDDeviceSetReport failed") &&
		!strings.Contains(message, "0xE00002BC") {
		return nil
	}
	frame, ok := deviceproto.ParseFrame(report)
	if !ok {
		return nil
	}
	switch frame.Command {
	case deviceproto.CmdSetGearRPM, deviceproto.CmdRGBEnable, deviceproto.CmdGearLight:
		// Flash-writing commands need a longer pause while the controller
		// finishes erasing/programming its data page.
		return []time.Duration{100 * time.Millisecond, 250 * time.Millisecond}
	case deviceproto.CmdSetRealtimeRPM, deviceproto.CmdEnterRealtimeRPM:
		// Realtime target updates are idempotent; one quick retry prevents a
		// missed fan-control cycle when the controller is momentarily busy.
		return []time.Duration{50 * time.Millisecond}
	default:
		return nil
	}
}
