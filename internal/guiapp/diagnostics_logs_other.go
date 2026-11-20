//go:build !linux && !darwin

package guiapp

import "archive/zip"

func addPlatformDiagnosticLogs(_ *zip.Writer) error {
	return nil
}
