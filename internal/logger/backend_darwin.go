//go:build darwin

package logger

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func newPlatformBackend(atom zap.AtomicLevel, installDir, identifier string, persistent bool) (platformBackend, error) {
	logDir := defaultLogDir(installDir)
	if logDir == "" {
		return platformBackend{
			cores: []zapcore.Core{consoleCore(atom, zapcore.AddSync(os.Stderr), isTerminal(os.Stderr))},
		}, nil
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return platformBackend{
			cores: []zapcore.Core{consoleCore(atom, zapcore.AddSync(os.Stderr), isTerminal(os.Stderr))},
		}, nil
	}

	name := sanitizeLogName(identifier)
	if name == "" {
		name = "thrm"
	}
	file := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, name+".log"),
		MaxSize:    10,
		MaxBackups: 7,
		MaxAge:     7,
		Compress:   true,
	}

	cores := []zapcore.Core{
		zapcore.NewCore(zapcore.NewJSONEncoder(logEncoderConfig()), zapcore.AddSync(file), atom),
	}
	// A GUI launched from Finder has stderr connected to /dev/null; adding the
	// console core only when it is a real terminal keeps that file clean while
	// still giving useful output when the binary is started from a shell.
	if isTerminal(os.Stderr) {
		cores = append(cores, consoleCore(atom, zapcore.AddSync(os.Stderr), true))
	}

	return platformBackend{
		cores:   cores,
		logDir:  logDir,
		closers: []io.Closer{file},
	}, nil
}

func defaultLogDir(_ string) string {
	homeDir, _ := os.UserHomeDir()
	return appmeta.UserLogDir(homeDir)
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func sanitizeLogName(value string) string {
	value = strings.TrimSpace(value)
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, value)
}
