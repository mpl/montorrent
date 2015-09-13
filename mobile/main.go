// Copyright 2015 - Mathieu Lonjaret

// +build darwin linux

package main

import (
	"log"

	"golang.org/x/mobile/app"
	"golang.org/x/mobile/asset"
)

var (
	cfg Config
)

func main() {
/*
	app.Main(func(a app.App) {
		var sz size.Event
		for e := range a.Events() {
			switch e := app.Filter(e).(type) {
			case lifecycle.Event:
				switch e.Crosses(lifecycle.StageVisible) {
				case lifecycle.CrossOn:
					onStart()
				case lifecycle.CrossOff:
					onStop()
				}
			case size.Event:
				sz = e
				touchX = float32(sz.WidthPx / 2)
				touchY = float32(sz.HeightPx / 2)
			case paint.Event:
				onPaint(sz)
				a.EndPaint(e)
			case touch.Event:
				touchX = e.X
				touchY = e.Y
			}
		}
	})
*/
}

type Config struct {
	Server string
	User	string
	Password string
}

func onStart() {
	a, err := asset.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	data, err := ioutil.ReadAll(a)
	if err != nil {
		log.Fatal(err)
	}
	
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}

	// Example code:
	// golang.org/x/mobile/example/basic/main.go
	// golang.org/x/mobile/example/sprite/main.go
	// TODO(mpl): Looks like I can't show text except by drawing it as glyphs with opengl (as it's done in https://github.com/golang/mobile/blob/master/exp/app/debug/fps.go#L31), which is super lame.
	// So maybe go the java bindings route ? sucks.
	
}

func onStop() {
}


