// Segra is a native segmented download manager. This package only embeds the
// menu bar icon and starts the desktop app; everything else lives in
// internal/, see docs/architecture.md.
package main

import (
	_ "embed"

	"github.com/beriholic/segra/internal/app"
)

//go:embed resources/tray.png
var trayPNG []byte

func main() { app.Run(trayPNG) }
