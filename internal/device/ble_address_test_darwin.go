//go:build darwin

package device

import (
	"strings"
	"testing"

	"tinygo.org/x/bluetooth"
)

// macOS identifies peripherals by UUID rather than a MAC address, so tests
// derive a stable UUID from the fixture's MAC to keep comparisons meaningful.
func mustBLEAddress(t *testing.T, mac string) bluetooth.Address {
	t.Helper()
	hex := strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
	if len(hex) != 12 {
		t.Fatalf("invalid test MAC %q", mac)
	}
	parsed, err := bluetooth.ParseUUID("00000000-0000-0000-0000-" + hex)
	if err != nil {
		t.Fatalf("ParseUUID(%q): %v", mac, err)
	}
	return bluetooth.Address{UUID: parsed}
}
