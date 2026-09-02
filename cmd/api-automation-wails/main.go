package main

import (
	"embed"
	"io/fs"
	"log"

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
	service := appwails.NewService(fiery.DefaultSecretKey)
	app := wails.New(wails.Options{
		Name:        "API Automation Preview",
		Description: "Read-only Wails 3 preview for Fiery API Automation",
		Services: []wails.Service{
			wails.NewService(service),
		},
		Assets: wails.AssetOptions{Handler: wails.BundledAssetFileServer(assets)},
		Mac: wails.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.Window.NewWithOptions(wails.WebviewWindowOptions{
		Name:            "api-automation-preview",
		Title:           "API Automation · Wails Preview",
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
