//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/types"
)

// MethodDesktop is kept as the reported method on macOS as well: the frontend
// already treats any non-Windows platform as "an autostart entry", and the
// underlying mechanism here is a per-user LaunchAgent plist.
const MethodDesktop = "desktop"

const launchAgentLabel = "top.tianli0.thrm.core"

type Manager struct {
	logger types.Logger
}

func NewManager(logger types.Logger) *Manager {
	return &Manager{logger: logger}
}

func launchAgentPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func (m *Manager) IsRunningAsAdmin() bool {
	return os.Geteuid() == 0
}

func (m *Manager) SetWindowsAutoStart(enable bool) error {
	return m.SetAutoStartWithMethod(enable, MethodDesktop)
}

func (m *Manager) GetAutoStartMethod() string {
	if autostartEnabled(launchAgentPath()) {
		return MethodDesktop
	}
	return "none"
}

// SetAutoStartWithMethod writes or removes a per-user LaunchAgent. method is
// accepted only for API parity with Windows; there is exactly one mechanism on
// macOS.
func (m *Manager) SetAutoStartWithMethod(enable bool, method string) error {
	plistPath := launchAgentPath()
	if plistPath == "" {
		return fmt.Errorf("无法确定用户主目录")
	}
	if !enable {
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 LaunchAgent 失败: %w", err)
		}
		// Do not bootout the running core: disabling autostart must not stop
		// fan control in the current session. launchd drops the job at logout.
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取核心服务路径失败: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("解析核心服务路径失败: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	home, _ := os.UserHomeDir()
	logDir := appmeta.UserLogDir(home)
	if logDir != "" {
		_ = os.MkdirAll(logDir, 0755)
	}
	stdoutPath := filepath.Join(logDir, "core-stdout.log")
	stderrPath := filepath.Join(logDir, "core-stderr.log")

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--autostart</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchAgentLabel, xmlEscape(exePath), xmlEscape(stdoutPath), xmlEscape(stderrPath))

	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 LaunchAgent 失败: %w", err)
	}
	return nil
}

func (m *Manager) CheckWindowsAutoStart() bool {
	return autostartEnabled(launchAgentPath())
}

// EnsureAutoStartTaskHealthy rewrites a stale LaunchAgent after the app was
// moved (e.g. dragged from Downloads into /Applications). Returns true when the
// plist was updated.
func (m *Manager) EnsureAutoStartTaskHealthy() (bool, error) {
	plistPath := launchAgentPath()
	if plistPath == "" {
		return false, nil
	}
	data, err := os.ReadFile(plistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	exePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	exePath, _ = filepath.Abs(exePath)
	if strings.Contains(string(data), "<string>"+xmlEscape(exePath)+"</string>") {
		return false, nil
	}
	if err := m.SetAutoStartWithMethod(true, MethodDesktop); err != nil {
		return false, err
	}
	return true, nil
}

// autostartEnabled reports whether the plist exists and points at an existing
// executable. A stale plist must not be reported as enabled.
func autostartEnabled(path string) bool {
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "<string>/") || !strings.HasSuffix(line, "</string>") {
			continue
		}
		program := strings.TrimSuffix(strings.TrimPrefix(line, "<string>"), "</string>")
		if info, err := os.Stat(program); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func DetectAutoStartLaunch(args []string) bool {
	for _, arg := range args {
		if arg == "--autostart" || arg == "/autostart" || arg == "-autostart" {
			return true
		}
	}
	return false
}

// xmlEscape escapes text for use inside an XML text node. Plist values we write
// are file paths, but a user name may legitimately contain '&' or '<'.
func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
