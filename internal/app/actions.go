package app

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

// clipboardLink returns the clipboard's text when it holds a bare http(s)
// link, so a stray sentence copied earlier never lands in the link field.
func clipboardLink(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, " \t\r\n") {
		return ""
	}
	u, err := url.Parse(text)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return text
}

func (a *App) openNew() {
	dir := a.settings.DownloadDir
	if dir == "" || dir == "." {
		if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
			dir = downloads
		}
	}
	a.newOpen = true
	a.newRun++
	a.newLink = ""
	if a.settings.ClipboardFill {
		a.newLink = clipboardLink(mygo.Clipboard.ReadText())
	}
	a.newDir = dir
	a.newConns = float64(a.settings.Connections)
	a.newErr = ""
	a.invalidate()
}

func (a *App) submitNew() {
	link := strings.TrimSpace(a.newLink)
	if link == "" {
		a.newErr = locales.UrlRequired()
		return
	}
	_, err := a.mgr.Create(download.CreateRequest{
		URL:         link,
		Dir:         a.newDir,
		Connections: int(a.newConns),
	})
	if err != nil {
		a.newErr = err.Error()
		return
	}
	a.newOpen = false
	a.newLink = ""
	a.newErr = ""
}

func (a *App) browseNewDir() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       locales.ChooseFolder(),
		DefaultPath: a.newDir,
		Directory:   true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := paths[0]
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() { a.newDir = dir })
	}
}

func (a *App) browseSettingsDir() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       locales.ChooseFolder(),
		DefaultPath: a.dirField,
		Directory:   true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := paths[0]
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Update(func() { a.dirField = dir })
	}
}

func (a *App) toggle(t download.Task) {
	switch t.Status {
	case download.StatusDownloading, download.StatusQueued:
		a.mgr.Pause(t.ID)
	case download.StatusPaused, download.StatusFailed:
		a.mgr.Resume(t.ID)
	}
}

func (a *App) remove(t download.Task, deleteFiles bool) {
	a.mgr.Remove(t.ID, deleteFiles)
	a.persistAsync()
}

func (a *App) reveal(t download.Task) {
	path := t.Path()
	if _, err := os.Stat(path); err == nil {
		mygo.Shell.ShowItemInFolder(path)
		return
	}
	mygo.Shell.OpenPath(t.Dir)
}

func (a *App) openDownloadFolder() {
	dir := a.settings.DownloadDir
	if dir == "" || dir == "." {
		if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
			dir = downloads
		}
	}
	mygo.Shell.OpenPath(dir)
}

func (a *App) openTask(t download.Task) {
	if t.Status == download.StatusCompleted {
		mygo.Shell.OpenPath(t.Path())
	}
}

func (a *App) setLanguage(tag string) {
	a.settings.Language = tag
	a.langTag = tag
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
	mygo.App.SetLocale(tag)
	a.buildMenu()
}

func (a *App) applySettings(c *ui.Context) {
	s := a.settings
	s.DownloadDir = filepath.Clean(strings.TrimSpace(a.dirField))
	if s.DownloadDir == "" || s.DownloadDir == "." {
		if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
			s.DownloadDir = downloads
		}
	}
	s.Connections = int(a.connsField)
	s.MaxConcurrent = int(a.concurrentField)
	s.APIPort = int(a.portField)
	s.Normalize()

	portChanged := s.APIPort != a.settings.APIPort
	a.settings = s
	a.syncFields()

	if err := a.store.SaveSettings(s); err != nil {
		log.Printf("save settings: %v", err)
	}
	a.mgr.SetConfig(download.Config{
		Dir:           s.DownloadDir,
		Connections:   s.Connections,
		MaxConcurrent: s.MaxConcurrent,
	})
	// The switch writes a.settings as it is flipped, so there is no old
	// value left to compare with: what the system was told last is the
	// only thing that says whether it needs telling again.
	if s.OpenAtLogin != a.loginSaved {
		if err := mygo.App.SetOpenAtLogin(s.OpenAtLogin); err != nil {
			log.Printf("open at login: %v", err)
			c.Toast(err.Error())
		} else {
			a.loginSaved = s.OpenAtLogin
		}
	}
	// The switches write a.settings as they are flipped, so compare with the
	// server that is actually running rather than with the old settings.
	running := a.apiPort > 0
	switch {
	case !s.EnableAPI && running:
		a.stopAPI()
	case s.EnableAPI && (!running || portChanged):
		a.restartAPI()
		if a.apiErr != "" {
			c.Toast(a.apiErr)
			return
		}
		if running && portChanged {
			c.Toast(locales.PortChanged())
			return
		}
	}
	c.Toast(locales.Saved())
}

func (a *App) syncFields() {
	a.dirField = a.settings.DownloadDir
	a.connsField = float64(a.settings.Connections)
	a.concurrentField = float64(a.settings.MaxConcurrent)
	a.portField = float64(a.settings.APIPort)
	a.langTag = a.settings.Language
}
