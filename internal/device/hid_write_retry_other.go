//go:build !darwin

package device

import "time"

// hidWriteRetryDelays is only needed for macOS' synchronous IOHIDDeviceSetReport.
// Windows and Linux hidapi implementations do not report a busy flash write as
// a failed output report.
func hidWriteRetryDelays([]byte, error) []time.Duration { return nil }
