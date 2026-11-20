//go:build darwin

package main

import (
	"os"

	"github.com/TIANLI0/THRM/internal/coreapp"
)

// runEventLoop drives the AppKit run loop on the main goroutine. Power
// notifications and the menu bar item both rely on it. A small watcher turns
// signals and tray quit requests into a clean shutdown, which ends the loop.
func runEventLoop(app *coreapp.CoreApp, sigChan <-chan os.Signal) {
	done := make(chan struct{})
	go func() {
		select {
		case <-sigChan:
			app.LogInfo("收到系统退出信号")
		case <-app.QuitChan():
			app.LogInfo("收到应用退出请求")
		case <-done:
			return
		}
		app.Stop()
	}()

	app.RunTrayMainLoop()
	close(done)
}
