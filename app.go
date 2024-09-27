package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	g "xabbo.b7c.io/goearth"
)

//go:embed all:frontend/dist
var assets embed.FS

var ext = g.NewExt(g.ExtInfo{
	Title:       "[AIO] Gamba Suite",
	Description: "Pkr, 13/21 and Tri dice automated rolling and resetting with in-chat hand evaluation. The all-in-one dice management plugin.",
	Version:     "2.0.1",
	Author:      "JTD",
})

var (
	CurrentVersion = "2.0.1"
)

var app *App

func (a *App) GetCurrentVersion() string {
	return CurrentVersion
}

func main() {
	app = NewApp(ext, assets)
	setupExt()
	err := wails.Run(&options.App{
		Title:  "[AIO] Gamba Suite by JTD",
		Width:  500,
		Height: 890,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 44, G: 62, B: 80, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		StartHidden:       true,
		HideWindowOnClose: true,
		DisableResize:     true,
		MinWidth:          500,
		MaxWidth:          500,
		MinHeight:         890,
		MaxHeight:         890,
	})

	if err != nil {
		log.Fatal(err)
	}
}

func setupExt() {
	ext.Initialized(func(e g.InitArgs) {
		log.Printf("initialized (connected=%t)", e.Connected)
	})

	ext.Activated(func() {
		log.Printf("activated")
		app.ShowWindow()
	})

	ext.Connected(func(e g.ConnectArgs) {
		log.Printf("connected (%s:%d)", e.Host, e.Port)
		log.Printf("client %s (%s)", e.Client.Identifier, e.Client.Version)
	})

	ext.Disconnected(func() {
		log.Printf("connection lost")
	})
}
