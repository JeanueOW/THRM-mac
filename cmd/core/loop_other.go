//go:build !darwin

package main

import (
	"os"

	"github.com/TIANLI0/THRM/internal/coreapp"
)

func runEventLoop(app *coreapp.CoreApp, sigChan <-chan os.Signal) {
	select {
	case <-sigChan:
		app.LogInfo("收到系统退出信号")
	case <-app.QuitChan():
		app.LogInfo("收到应用退出请求")
	}
}
