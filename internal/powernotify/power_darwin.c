//go:build darwin

#include <IOKit/pwr_mgt/IOPMLib.h>
#include <IOKit/IOMessage.h>
#include <CoreFoundation/CoreFoundation.h>

extern void thrmPowerEvent(int kind);

static io_connect_t gRootPort = 0;
static IONotificationPortRef gNotifyPort = NULL;
static io_object_t gNotifier = 0;

static void thrmPowerCallback(void *refCon, io_service_t service, natural_t messageType, void *messageArgument) {
    switch (messageType) {
    case kIOMessageCanSystemSleep:
        // Allow the sleep to proceed. The actual suspend cleanup happens on
        // kIOMessageSystemWillSleep.
        IOAllowPowerChange(gRootPort, (long)messageArgument);
        break;
    case kIOMessageSystemWillSleep:
        thrmPowerEvent(0);
        IOAllowPowerChange(gRootPort, (long)messageArgument);
        break;
    case kIOMessageSystemHasPoweredOn:
        thrmPowerEvent(1);
        break;
    default:
        break;
    }
}

int thrmRegisterPowerNotifications(void) {
    if (gRootPort != 0) {
        return 0;
    }
    gRootPort = IORegisterForSystemPower(NULL, &gNotifyPort, thrmPowerCallback, &gNotifier);
    if (gRootPort == 0) {
        return -1;
    }
    // The core's main thread runs the NSApplication/Carbon event loop on
    // macOS, which is also a CFRunLoop; power messages arrive there.
    CFRunLoopAddSource(CFRunLoopGetMain(), IONotificationPortGetRunLoopSource(gNotifyPort), kCFRunLoopCommonModes);
    return 0;
}

void thrmUnregisterPowerNotifications(void) {
    if (gNotifier != 0) {
        IODeregisterForSystemPower(&gNotifier);
        gNotifier = 0;
    }
    if (gNotifyPort != NULL) {
        IONotificationPortDestroy(gNotifyPort);
        gNotifyPort = NULL;
    }
    if (gRootPort != 0) {
        IOServiceClose(gRootPort);
        gRootPort = 0;
    }
}
