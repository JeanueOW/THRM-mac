//go:build darwin

package device

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreBluetooth
#import <CoreBluetooth/CoreBluetooth.h>

static int thrmBluetoothAuthorization(void) {
    if (@available(macOS 10.15, *)) {
        return (int)CBManager.authorization;
    }
    return -1;
}
*/
import "C"

import "strings"

const (
	cbAuthNotDetermined = 0
	cbAuthRestricted    = 1
	cbAuthDenied        = 2
	cbAuthAllowed       = 3
)

// bleAuthorizationHint returns a user-facing explanation when macOS has
// already denied Bluetooth access. Without this check, tinygo's Enable() waits
// the full 10 second timeout whenever CBCentralManager was created before a
// delegate was attached (the state never changes, so no callback arrives).
func bleAuthorizationHint() string {
	switch int(C.thrmBluetoothAuthorization()) {
	case cbAuthDenied, cbAuthRestricted:
		return "macOS 未授予蓝牙权限。请在“系统设置 → 隐私与安全性 → 蓝牙”中允许 THRM；" +
			"未签名的本地构建通常不会弹出授权框，请使用 Developer ID 签名，或先在终端运行 thrm-core 再启动 GUI"
	case cbAuthNotDetermined:
		// Let Enable() trigger the system prompt.
		return ""
	default:
		return ""
	}
}

// bleEnableFailureHint explains the common macOS 26 failure mode where the
// system refuses to show a Bluetooth permission prompt for an ad-hoc signed
// bundle. CBCentralManager then never leaves its initial state and tinygo
// reports a timeout.
func bleEnableFailureHint(err error) string {
	if err == nil || !strings.Contains(err.Error(), "timeout enabling CentralManager") {
		return ""
	}
	return "macOS 没有弹出蓝牙授权框。未签名的本地构建会被系统直接拒绝蓝牙权限：" +
		"请使用 Developer ID 签名后重新构建，或在“系统设置 → 隐私与安全性 → 蓝牙”中手动添加 THRM"
}
