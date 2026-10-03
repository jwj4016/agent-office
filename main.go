package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Agent Office",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 248, B: 236, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// A second launch focuses the running window instead of starting
		// another engine on the same data (the data dir lock is the
		// backstop for other launch paths).
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.agent-office.desktop",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
