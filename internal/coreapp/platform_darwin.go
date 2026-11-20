//go:build darwin

package coreapp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TIANLI0/THRM/internal/appmeta"
)

func (a *CoreApp) ReinstallPawnIO() (map[string]any, error) {
	return map[string]any{
		"success": false,
		"message": "PawnIO is only used by the Windows temperature bridge and is not supported on macOS",
	}, nil
}

// launchGUI opens the app bundle through LaunchServices when the core lives
// inside THRM.app, which gives normal activation and Dock behaviour. The direct
// executable path remains as a fallback for unbundled development builds.
func launchGUI() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	if bundle := appBundlePath(exeDir); bundle != "" {
		// -n avoids LaunchServices treating a just-quit instance as still
		// running and silently ignoring the request. Wails' single-instance
		// lock makes a redundant launch harmless: it just shows the window.
		if err := exec.Command("open", "-n", bundle).Start(); err == nil {
			return nil
		}
	}

	candidates := appmeta.GUIExecutableCandidates(exeDir)
	guiPath := appmeta.FirstExistingPath(candidates)
	if guiPath == "" {
		if path, err := exec.LookPath(appmeta.ExecutableName); err == nil {
			guiPath = path
		}
	}
	if guiPath == "" {
		return fmt.Errorf("GUI executable not found: %v", candidates)
	}
	cmd := exec.Command(guiPath)
	cmd.Dir = filepath.Dir(guiPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start GUI: %w", err)
	}
	return cmd.Process.Release()
}

// appBundlePath returns the .app directory that contains dir, if any.
func appBundlePath(dir string) string {
	clean := filepath.Clean(dir)
	for {
		if strings.HasSuffix(clean, ".app") {
			return clean
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return ""
		}
		clean = parent
	}
}
