//go:build darwin

#include <Carbon/Carbon.h>
#include <stdint.h>

extern void thrmHotkeyCallback(int id);

static EventHandlerRef gHotKeyHandler = NULL;
static EventHotKeyRef gHotKeyRefs[8] = {0};

static OSStatus thrmHotKeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    EventHotKeyID hotKeyID;
    OSStatus status = GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID,
                                        NULL, sizeof(hotKeyID), NULL, &hotKeyID);
    if (status == noErr && hotKeyID.signature == 'THRM') {
        thrmHotkeyCallback((int)hotKeyID.id);
    }
    return noErr;
}

static void thrmEnsureHotKeyHandler(void) {
    if (gHotKeyHandler != NULL) {
        return;
    }
    EventTypeSpec spec = { kEventClassKeyboard, kEventHotKeyPressed };
    InstallEventHandler(GetApplicationEventTarget(), NewEventHandlerUPP(thrmHotKeyHandler),
                        1, &spec, NULL, &gHotKeyHandler);
}

int thrmHotKeyRegister(int id, uint32_t modifiers, uint32_t keyCode) {
    if (id < 0 || id >= (int)(sizeof(gHotKeyRefs) / sizeof(gHotKeyRefs[0]))) {
        return paramErr;
    }
    thrmEnsureHotKeyHandler();

    if (gHotKeyRefs[id] != NULL) {
        UnregisterEventHotKey(gHotKeyRefs[id]);
        gHotKeyRefs[id] = NULL;
    }

    EventHotKeyID hotKeyID;
    hotKeyID.signature = 'THRM';
    hotKeyID.id = (UInt32)id;

    return RegisterEventHotKey((UInt32)keyCode, modifiers, hotKeyID,
                               GetApplicationEventTarget(), 0, &gHotKeyRefs[id]);
}

int thrmHotKeyUnregister(int id) {
    if (id < 0 || id >= (int)(sizeof(gHotKeyRefs) / sizeof(gHotKeyRefs[0]))) {
        return paramErr;
    }
    if (gHotKeyRefs[id] == NULL) {
        return noErr;
    }
    OSStatus status = UnregisterEventHotKey(gHotKeyRefs[id]);
    gHotKeyRefs[id] = NULL;
    return status;
}
