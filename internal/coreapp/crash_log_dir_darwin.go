//go:build darwin

package coreapp

import (
	"os"

	"github.com/TIANLI0/THRM/internal/appmeta"
)

// platformCrashLogDir keeps crash reports next to the rotating log files in
// ~/Library/Logs/THRM, where Console.app and the diagnostic export can find
// them without requiring special permissions.
func platformCrashLogDir() string {
	homeDir, _ := os.UserHomeDir()
	dir := appmeta.UserLogDir(homeDir)
	if dir == "" {
		return ""
	}
	_ = os.MkdirAll(dir, 0755)
	return dir
}
