//go:build darwin

package coreapp

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/temperature"
	"github.com/TIANLI0/THRM/internal/types"
)

func temperatureHistoryPath(string) string {
	homeDir, _ := os.UserHomeDir()
	stateDir := appmeta.UserStateDir(homeDir)
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, temperature.DefaultHistoryRelativePath)
}

// migrateLegacyTemperatureHistory preserves data from a previous portable
// build. macOS has no old official package, but users may have run an earlier
// development build from the Downloads folder.
func migrateLegacyTemperatureHistory(destination, installDir string, logger types.Logger) {
	if destination == "" {
		return
	}
	if _, err := os.Stat(destination); err == nil {
		return
	}

	candidates := []string{filepath.Join(installDir, temperature.DefaultHistoryRelativePath)}
	for _, source := range candidates {
		if source == destination {
			continue
		}
		info, err := os.Stat(source)
		if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
			continue
		}
		payload, err := os.ReadFile(source)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return
		}
		destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return
		}
		_, writeErr := destinationFile.Write(payload)
		closeErr := destinationFile.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(destination)
			return
		}
		if logger != nil {
			logger.Info("已将温度历史从旧安装目录迁移到: %s", destination)
		}
		return
	}
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
