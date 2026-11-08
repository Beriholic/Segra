package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/beriholic/segra/extension"
	"github.com/beriholic/segra/locales"
)

// extensionConfig is what an exported copy knows before the user touches
// its options page.
type extensionConfig struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// extensionZip writes the embedded extension to w, with a config.json of
// the port and token it runs with: a copy loaded from the zip works at once,
// where one loaded from the repository asks for both.
func extensionZip(w io.Writer, port int, token string) error {
	zw := zip.NewWriter(w)
	err := fs.WalkDir(extension.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := extension.Files.ReadFile(path)
		if err != nil {
			return err
		}
		return zipFile(zw, path, data)
	})
	if err == nil {
		config, err2 := json.MarshalIndent(extensionConfig{Port: port, Token: token}, "", "  ")
		if err2 != nil {
			err = err2
		} else {
			err = zipFile(zw, "config.json", append(config, '\n'))
		}
	}
	// Closing the writer writes the central directory, so a zip that failed
	// halfway must not leave it half written to the caller either.
	if err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// zipFile adds one file to the archive, readable by the browser that loads
// it and stamped with today rather than the zero time an embed carries.
func zipFile(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()}
	header.SetMode(0o644)
	f, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// exportExtension saves the extension as a zip, which the user unzips and
// loads unpacked: the Chrome Web Store is the only place Chrome installs a
// zip from, and Segra is not in it. port and token come from the frame that
// asked, as the settings are the view's to write.
func (a *App) exportExtension(port int, token string) {
	name := "segra-extension"
	if v := extension.Version; v != "" {
		name += "-" + v
	}
	dir, err := mygo.App.Path(mygo.PathDownloads)
	if err != nil {
		dir, _ = os.UserHomeDir()
	}

	a.mu.Lock()
	win := a.win
	a.mu.Unlock()

	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      win,
		Title:       locales.ExportExtensionTitle(),
		DefaultPath: filepath.Join(dir, name+".zip"),
		Filters:     []mygo.FileFilter{{Name: "Zip", Extensions: []string{"zip"}}},
	})
	if err != nil || path == "" {
		return
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		path += ".zip"
	}

	var buf bytes.Buffer
	if err := extensionZip(&buf, port, token); err != nil {
		a.say(err.Error())
		return
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		a.say(err.Error())
		return
	}
	mygo.Shell.ShowItemInFolder(path)
	a.say(locales.ExtensionExported())
}
