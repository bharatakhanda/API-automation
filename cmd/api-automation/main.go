package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"api-automation/internal/appwails"
	"api-automation/internal/fiery"

	wails "github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed frontend/bundle
var frontend embed.FS

func main() {
	assets, err := fs.Sub(frontend, "frontend/bundle")
	if err != nil {
		log.Fatal(err)
	}
	dialogs := new(nativeDialogs)
	var app *wails.App
	service := appwails.NewService(fiery.DefaultSecretKey, appwails.Options{
		Dialogs:        dialogs,
		DebugDirectory: executableDebugDirectory(),
		EventEmitter: func(name string, data any) {
			if app != nil {
				app.Event.Emit(name, data)
			}
		},
	})
	shutdown := sync.OnceFunc(func() { appwails.Shutdown(service) })
	defer shutdown()
	app = wails.New(wails.Options{
		Name:        "API Automation",
		Description: "Fiery API Automation desktop application",
		Services: []wails.Service{
			wails.NewService(service),
		},
		Assets:     wails.AssetOptions{Handler: wails.BundledAssetFileServer(assets)},
		OnShutdown: shutdown,
		Mac: wails.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	dialogs.app = app
	app.Window.NewWithOptions(wails.WebviewWindowOptions{
		Name:            "api-automation",
		Title:           "API Automation",
		URL:             "/",
		Width:           1240,
		Height:          820,
		MinWidth:        920,
		MinHeight:       620,
		DevToolsEnabled: false,
		BackgroundColour: wails.RGBA{
			Red: 245, Green: 247, Blue: 251, Alpha: 255,
		},
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func executableDebugDirectory() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	executable, err := os.Executable()
	if err != nil {
		log.Printf("Wails debug directory unavailable: %v", err)
		return ""
	}
	return filepath.Dir(executable)
}
