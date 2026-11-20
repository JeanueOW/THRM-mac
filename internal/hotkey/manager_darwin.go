//go:build darwin

package hotkey

/*
#cgo LDFLAGS: -framework Carbon
#include <stdint.h>

extern void thrmHotkeyCallback(int id);

int thrmHotKeyRegister(int id, uint32_t modifiers, uint32_t keyCode);
int thrmHotKeyUnregister(int id);
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/TIANLI0/THRM/internal/logger"
	"github.com/TIANLI0/THRM/internal/types"
)

// Action 定义快捷键触发的动作。
type Action string

const (
	ActionToggleManualGear   Action = "toggle-manual-gear"
	ActionToggleAutoMode     Action = "toggle-auto-control"
	ActionToggleCurveProfile Action = "toggle-curve-profile"
)

// Carbon modifier masks (Events.h). These do not require Accessibility
// permission, unlike CGEventTap based implementations.
type carbonModifier uint32

const (
	modCmd     carbonModifier = 1 << 8
	modShift   carbonModifier = 1 << 9
	modOption  carbonModifier = 1 << 11
	modControl carbonModifier = 1 << 12
)

type binding struct {
	shortcut string
	id       int
}

type Manager struct {
	logger   types.Logger
	onAction func(action Action, shortcut string)

	mutex    sync.Mutex
	bindings map[Action]*binding
	closed   bool
}

// activeManager receives the Carbon callback. Registration ids are fixed per
// action, so a single package-level pointer is enough; only one core process
// (and therefore one manager) exists.
var activeManager atomic.Pointer[Manager]

// actionForID maps the Carbon hot key id back to its action.
var (
	actionIDs = []Action{
		ActionToggleManualGear,
		ActionToggleAutoMode,
		ActionToggleCurveProfile,
	}
	actionIDValue = map[Action]int{
		ActionToggleManualGear:   1,
		ActionToggleAutoMode:     2,
		ActionToggleCurveProfile: 3,
	}
)

func NewManager(logger types.Logger, onAction func(action Action, shortcut string)) *Manager {
	m := &Manager{
		logger:   logger,
		onAction: onAction,
		bindings: make(map[Action]*binding),
	}
	activeManager.Store(m)
	return m
}

func (m *Manager) UpdateBindings(manualGearShortcut, autoControlShortcut, curveProfileShortcut string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return fmt.Errorf("hotkey manager already stopped")
	}

	var errs []string
	if err := m.setBindingLocked(ActionToggleManualGear, manualGearShortcut); err != nil {
		errs = append(errs, err.Error())
	}
	if err := m.setBindingLocked(ActionToggleAutoMode, autoControlShortcut); err != nil {
		errs = append(errs, err.Error())
	}
	if err := m.setBindingLocked(ActionToggleCurveProfile, curveProfileShortcut); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (m *Manager) setBindingLocked(action Action, shortcut string) error {
	shortcut = normalizeShortcut(shortcut)

	if existing, ok := m.bindings[action]; ok {
		if existing.shortcut == shortcut {
			return nil
		}
		m.unregisterLocked(action)
	}

	if shortcut == "" {
		return nil
	}

	mods, key, err := ParseShortcut(shortcut)
	if err != nil {
		return fmt.Errorf("invalid shortcut for %s: %w", action, err)
	}

	id := actionIDValue[action]
	if status := C.thrmHotKeyRegister(C.int(id), C.uint32_t(mods), C.uint32_t(key)); status != 0 {
		return fmt.Errorf("failed to register %s (%s): OSStatus %d（可能已被其他应用占用）", action, shortcut, int(status))
	}
	m.bindings[action] = &binding{shortcut: shortcut, id: id}
	m.logInfo("hotkey registered: %s -> %s", action, shortcut)
	return nil
}

func (m *Manager) unregisterLocked(action Action) {
	b, ok := m.bindings[action]
	if !ok || b == nil {
		return
	}
	C.thrmHotKeyUnregister(C.int(b.id))
	delete(m.bindings, action)
}

func (m *Manager) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return
	}
	m.closed = true
	for _, action := range actionIDs {
		m.unregisterLocked(action)
	}
	if activeManager.Load() == m {
		activeManager.Store(nil)
	}
}

func (m *Manager) dispatch(id int) {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return
	}
	var matched Action
	var shortcut string
	for action, b := range m.bindings {
		if b != nil && b.id == id {
			matched = action
			shortcut = b.shortcut
			break
		}
	}
	m.mutex.Unlock()

	if matched == "" {
		return
	}
	if m.onAction != nil {
		m.onAction(matched, shortcut)
	}
}

//export thrmHotkeyCallback
func thrmHotkeyCallback(id C.int) {
	if m := activeManager.Load(); m != nil {
		m.dispatch(int(id))
	}
}

// ParseShortcut parses "Ctrl+Alt+Shift+Win+M" style shortcuts and returns the
// Carbon modifier mask and virtual key code.
func ParseShortcut(input string) (carbonModifier, uint16, error) {
	normalized := normalizeShortcut(input)
	if normalized == "" {
		return 0, 0, fmt.Errorf("empty shortcut")
	}

	parts := strings.Split(normalized, "+")
	var mods carbonModifier
	key := uint16(0)
	keySet := false

	for _, part := range parts {
		if part == "" {
			continue
		}
		if mod, ok := parseModifier(part); ok {
			mods |= mod
			continue
		}
		if keySet {
			return 0, 0, fmt.Errorf("multiple main keys")
		}
		parsedKey, err := parseKey(part)
		if err != nil {
			return 0, 0, err
		}
		key = parsedKey
		keySet = true
	}

	if !keySet {
		return 0, 0, fmt.Errorf("missing main key")
	}
	if mods == 0 {
		return 0, 0, fmt.Errorf("missing modifier")
	}
	return mods, key, nil
}

func parseModifier(part string) (carbonModifier, bool) {
	switch strings.ToUpper(strings.TrimSpace(part)) {
	case "CTRL", "CONTROL":
		return modControl, true
	case "ALT":
		return modOption, true
	case "SHIFT":
		return modShift, true
	case "WIN", "WINDOWS", "SUPER", "CMD", "COMMAND", "META":
		return modCmd, true
	default:
		return 0, false
	}
}

// Key codes from Carbon/Events.h (kVK_*). UI recording only produces letters,
// digits and F1-F12, which matches the Windows/Linux behaviour.
var keyCodes = map[string]uint16{
	"A": 0x00, "S": 0x01, "D": 0x02, "F": 0x03, "H": 0x04, "G": 0x05,
	"Z": 0x06, "X": 0x07, "C": 0x08, "V": 0x09, "B": 0x0B, "Q": 0x0C,
	"W": 0x0D, "E": 0x0E, "R": 0x0F, "Y": 0x10, "T": 0x11,
	"1": 0x12, "2": 0x13, "3": 0x14, "4": 0x15, "6": 0x16, "5": 0x17,
	"9": 0x19, "7": 0x1A, "8": 0x1C, "0": 0x1D,
	"O": 0x1F, "U": 0x20, "I": 0x22, "P": 0x23, "L": 0x25, "J": 0x26,
	"K": 0x28, "N": 0x2D, "M": 0x2E,
	"F1": 0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76, "F5": 0x60, "F6": 0x61,
	"F7": 0x62, "F8": 0x64, "F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F,
}

func parseKey(part string) (uint16, error) {
	token := strings.ToUpper(strings.TrimSpace(part))
	if code, ok := keyCodes[token]; ok {
		return code, nil
	}
	if after, ok := strings.CutPrefix(token, "F"); ok {
		if n, err := strconv.Atoi(after); err == nil {
			return 0, fmt.Errorf("unsupported key: F%d", n)
		}
	}
	return 0, fmt.Errorf("unsupported key: %s", part)
}

func normalizeShortcut(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	input = strings.ReplaceAll(input, " ", "")
	input = strings.ReplaceAll(input, "-", "+")
	input = strings.ReplaceAll(input, "_", "+")

	parts := strings.Split(input, "+")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, strings.ToUpper(p))
	}
	return strings.Join(out, "+")
}

func (m *Manager) logInfo(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Info(format, v...)
	}
}
