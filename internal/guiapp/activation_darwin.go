//go:build darwin

package guiapp

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void thrmApplyActivationPolicy(int policy) {
    // Regular = 0, Accessory = 1 (NSApplicationActivationPolicy).
    void (^apply)(void) = ^{
        [NSApp setActivationPolicy:(NSApplicationActivationPolicy)policy];
        if (policy == NSApplicationActivationPolicyRegular) {
            [NSApp activateIgnoringOtherApps:YES];
        }
    };
    if ([NSThread isMainThread]) {
        apply();
    } else {
        dispatch_async(dispatch_get_main_queue(), apply);
    }
}
*/
import "C"

// prepareWindowShow restores the Dock icon and Cmd-Tab entry before showing the
// window. While the window is closed the GUI runs as an accessory app, so it
// does not appear in the Dock, Cmd-Tab or Mission Control.
func prepareWindowShow() { C.thrmApplyActivationPolicy(0) }

// windowDidHide switches the GUI to accessory mode after its window is hidden.
func windowDidHide() { C.thrmApplyActivationPolicy(1) }
