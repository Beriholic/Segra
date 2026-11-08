// Package extension embeds the Chromium extension, so that the app can hand
// it out as a zip wherever Segra runs from: a copy of the repository is not
// something a user of the .dmg has.
package extension

import (
	"embed"
	"encoding/json"
)

// Files holds the extension as Chrome loads it, paths relative to this
// directory.
//
//go:embed manifest.json *.js *.html *.css icons all:_locales
var Files embed.FS

// Version is the extension's own version, from manifest.json, or "" when it
// cannot be read. It is read once: the manifest is embedded, and the view
// asks for it every frame.
var Version = manifestVersion()

func manifestVersion() string {
	data, err := Files.ReadFile("manifest.json")
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return m.Version
}
