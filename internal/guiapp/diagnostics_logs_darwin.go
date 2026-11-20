//go:build darwin

package guiapp

import (
	"archive/zip"
	"os"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/logger"
)

// addPlatformDiagnosticLogs includes the rotating GUI/core logs stored in
// ~/Library/Logs/THRM, plus any fatal_*.log files left by a crash.
func addPlatformDiagnosticLogs(archive *zip.Writer) error {
	homeDir, _ := os.UserHomeDir()
	logDir := appmeta.UserLogDir(homeDir)
	if logDir == "" {
		return nil
	}
	for _, identifier := range []string{logger.GUIIdentifier, logger.CoreIdentifier} {
		if err := addRecentDiagnosticLogs(archive, logDir, identifier, 8); err != nil {
			return err
		}
	}
	return addRecentDiagnosticLogs(archive, logDir, "fatal_", 4)
}
