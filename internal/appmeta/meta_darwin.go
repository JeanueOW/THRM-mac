//go:build darwin

package appmeta

import (
	"os"
	"path/filepath"
)

const (
	ExecutableName       = "THRM"
	LegacyExecutableName = "thrm"
	CoreExecutableName   = "thrm-core"
	LegacyCoreExecutable = "bs2pro-core"
	BridgeName           = ""
	BridgeExecutableName = ""
	PawnIOInstallerName  = ""
)

// UserConfigDir follows the macOS File System Programming Guide: application
// support data lives in ~/Library/Application Support/<AppName>. Keeping the
// directory name equal to the product name also makes it easy to find in
// Finder, and it never ends up inside the (read-only) app bundle.
func UserConfigDir(homeDir string) string {
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, "Library", "Application Support", AppName)
}

// UserStateDir is for persistent mutable data which is not configuration,
// such as temperature history. macOS has no separate XDG state directory, so
// it lives next to the configuration.
func UserStateDir(homeDir string) string {
	return UserConfigDir(homeDir)
}

// UserLogDir is where rotating logs and crash reports are written.
func UserLogDir(homeDir string) string {
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, "Library", "Logs", AppName)
}

func CoreExecutableCandidates(baseDir string) []string {
	candidates := []string{
		filepath.Join(baseDir, CoreExecutableName),
		filepath.Join(baseDir, LegacyCoreExecutable),
	}
	contentsDir := appBundleContentsDir(baseDir)
	if contentsDir != "" {
		candidates = append(candidates,
			filepath.Join(contentsDir, "MacOS", CoreExecutableName),
			filepath.Join(contentsDir, "MacOS", LegacyCoreExecutable),
			filepath.Join(contentsDir, "Resources", CoreExecutableName),
			filepath.Join(contentsDir, "Resources", LegacyCoreExecutable),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, CoreExecutableName),
			filepath.Join(exeDir, LegacyCoreExecutable),
			filepath.Join(exeDir, "..", "Resources", CoreExecutableName),
			filepath.Join(exeDir, "..", "Resources", LegacyCoreExecutable),
		)
	}
	return candidates
}

func GUIExecutableCandidates(baseDir string) []string {
	candidates := []string{
		filepath.Join(baseDir, ExecutableName),
		filepath.Join(baseDir, LegacyExecutableName),
	}
	contentsDir := appBundleContentsDir(baseDir)
	if contentsDir != "" {
		candidates = append(candidates,
			filepath.Join(contentsDir, "MacOS", ExecutableName),
			filepath.Join(contentsDir, "MacOS", LegacyExecutableName),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, ExecutableName),
			filepath.Join(exeDir, LegacyExecutableName),
			filepath.Join(exeDir, "..", "MacOS", ExecutableName),
			filepath.Join(exeDir, "..", "MacOS", LegacyExecutableName),
		)
	}
	return candidates
}

func BridgeExecutableCandidates(baseDir string) []string {
	return nil
}

func PawnIOInstallerPath(baseDir string) string {
	return ""
}

func PawnIOInstallerCandidates(baseDir string) []string {
	return nil
}

// appBundleContentsDir returns the Contents directory when dir looks like a
// path inside an .app bundle (either Contents/MacOS or Contents/Resources).
func appBundleContentsDir(dir string) string {
	if dir == "" {
		return ""
	}
	clean := filepath.Clean(dir)
	parent := filepath.Dir(clean)
	if filepath.Base(clean) == "MacOS" || filepath.Base(clean) == "Resources" {
		if filepath.Base(parent) == "Contents" {
			return parent
		}
	}
	if filepath.Base(clean) == "Contents" {
		return clean
	}
	return ""
}
