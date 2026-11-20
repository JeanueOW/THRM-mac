//go:build !darwin

package device

// bleAuthorizationHint is only needed on macOS, where CoreBluetooth access is
// gated by TCC. Other platforms either prompt in the driver (Windows) or use
// BlueZ permissions (Linux).
func bleAuthorizationHint() string { return "" }

func bleEnableFailureHint(error) string { return "" }
