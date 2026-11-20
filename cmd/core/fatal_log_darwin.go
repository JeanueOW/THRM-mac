//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/TIANLI0/THRM/internal/appmeta"
)

// setupFatalOutput redirects the Go runtime's fatal output into a per-launch
// file under ~/Library/Logs/THRM. The core is normally started by the GUI or by
// launchd, so stderr would otherwise be lost.
func setupFatalOutput() (func(), string) {
	debug.SetTraceback("all")

	homeDir, _ := os.UserHomeDir()
	logDir := appmeta.UserLogDir(homeDir)
	if logDir == "" {
		return func() {}, ""
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return func() {}, ""
	}
	filePath := filepath.Join(logDir, fmt.Sprintf("fatal_%s.log", time.Now().Format("2006-01-02_15-04-05.000")))
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return func() {}, ""
	}

	// dup2 makes the runtime's own writes (before zap is ready) land in the
	// file too; reassigning the os.File variables covers Go-level writes.
	if err := syscall.Dup2(int(file.Fd()), int(os.Stderr.Fd())); err != nil {
		_ = file.Close()
		return func() {}, ""
	}
	if err := syscall.Dup2(int(file.Fd()), int(os.Stdout.Fd())); err != nil {
		_ = file.Close()
		return func() {}, ""
	}
	os.Stderr = file
	os.Stdout = file

	return func() { _ = file.Close() }, filePath
}
