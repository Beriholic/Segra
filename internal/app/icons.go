package app

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Icons come from Lucide (https://lucide.dev, ISC): 24px stroke paths that
// take the text color of the element showing them, so one set serves every
// state.
//
//go:embed icons/*.svg
var iconFS embed.FS

func mustIcon(name string) *ui.SVG {
	b, err := iconFS.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic("icon " + name + ": " + err.Error())
	}
	return ui.MustParseSVG(b)
}

var (
	icDownload = mustIcon("download")
	icBolt     = mustIcon("zap")
	icCheck    = mustIcon("circle-check")
	icClock    = mustIcon("clock")
	icFolder   = mustIcon("folder")
	icGrid     = mustIcon("layout-grid")
	icFile     = mustIcon("file")
	icDisc     = mustIcon("disc")
	icPackage  = mustIcon("package")
	icVideo    = mustIcon("file-video")
	icArchive  = mustIcon("archive")
	icData     = mustIcon("database")
	icPause    = mustIcon("pause")
	icPlay     = mustIcon("play")
	icExternal = mustIcon("external-link")
	icPlus     = mustIcon("plus")
	icServer   = mustIcon("server")
	icFailed   = mustIcon("circle-x")
	icSettings = mustIcon("settings")
	icActive   = mustIcon("arrow-down")
	icChevronL = mustIcon("chevron-left")
	icChevronR = mustIcon("chevron-right")
)

func iconForName(name string) *ui.SVG {
	dot := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			dot = i
			break
		}
	}
	ext := ""
	if dot >= 0 {
		ext = name[dot+1:]
	}
	switch ext {
	case "iso", "img", "dmg":
		return icDisc
	case "apk", "exe", "msi", "deb", "rpm":
		return icPackage
	case "mp4", "mov", "mkv", "webm", "avi":
		return icVideo
	case "zip", "tar", "gz", "7z", "rar", "xz":
		return icArchive
	case "csv", "json", "db", "sqlite", "parquet":
		return icData
	default:
		return icFile
	}
}
