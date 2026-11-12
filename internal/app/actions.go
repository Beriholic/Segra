package app

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/internal/store"
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
		win.Update(func() {
			a.dirField = dir
			a.fieldDirty = false
			a.applySettings()
		})
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

// clearHistory drops the finished downloads in the given statuses from the
// list, which is the tidy-up the user asks for by hand. The records go and
// the files stay, exactly as the automatic clean-up leaves them.
func (a *App) clearHistory(statuses ...download.Status) {
	if a.mgr == nil {
		return
	}
	n := a.mgr.ClearStatus(false, statuses...)
	if n == 0 {
		return
	}
	a.persistAsync()
	a.say(locales.HistoryCleared())
}

// pruneHistory drops the finished records the history settings no longer
// keep: those past their days, and all but the newest the limit allows. Only
// the records go; the files stay where they were downloaded.
func (a *App) pruneHistory() int {
	if a.mgr == nil || !a.settings.HistoryAutoClean {
		return 0
	}
	keep := time.Duration(a.settings.HistoryDays) * 24 * time.Hour
	n := a.mgr.PruneHistory(keep, a.settings.HistoryLimit)
	if n > 0 {
		a.persistAsync()
	}
	return n
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

// setTheme switches the appearance and writes it down. The system's own
// chrome — the title bar, the menus, the dialogs — follows through
// mygo.Theme, so the window is not the only thing that turns dark.
func (a *App) setTheme(tag string) {
	a.settings.Theme = tag
	a.themePick = themeIndex(tag)
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
	if mygo.App.IsReady() {
		mygo.Theme.SetSource(mygo.ThemeSource(tag))
	}
	a.invalidate()
	a.invalidatePanel()
}

// applySettings makes the app run on what the settings view holds, and
// writes it down: there is no Save button, so every control calls this as
// it changes. A folder that is only half typed is skipped by the view, and
// a switch flips a setting whole.
func (a *App) applySettings() {
	s := a.captureSettings()
	portChanged := s.APIPort != a.settings.APIPort
	a.settings = s
	a.fieldDirty = false
	a.syncFields()

	if err := a.store.SaveSettings(s); err != nil {
		log.Printf("save settings: %v", err)
	}
	if a.mgr != nil {
		a.mgr.SetConfig(download.Config{
			Dir:           s.DownloadDir,
			Connections:   s.Connections,
			MaxConcurrent: s.MaxConcurrent,
		})
	}
	// The history settings just changed, so what they keep goes now rather
	// than at the next download: lowering a limit is meant to tidy the list
	// as it is typed.
	a.pruneHistory()
	// The switch writes a.settings as it is flipped, so there is no old
	// value left to compare with: what the system was told last is the
	// only thing that says whether it needs telling again.
	if s.OpenAtLogin != a.loginSaved {
		if err := mygo.App.SetOpenAtLogin(s.OpenAtLogin); err != nil {
			log.Printf("open at login: %v", err)
			a.say(err.Error())
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
			a.say(a.apiErr)
			return
		}
		if running && portChanged {
			a.say(locales.PortChanged())
			return
		}
	}
	a.say(locales.Saved())
}

// commitField saves the folder typed into the settings once its field is
// done with: until then its letters are not a folder a download could land
// in, and a path half typed is worse than the one before it.
func (a *App) commitField() {
	if !a.fieldDirty {
		return
	}
	a.fieldDirty = false
	a.applySettings()
}

// captureSettings folds the settings view's fields into the settings the
// app runs on, so that a folder typed into a field and one picked in a
// dialog both become the same setting.
func (a *App) captureSettings() store.Settings {
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
	s.HistoryDays = int(a.historyDaysField)
	s.HistoryLimit = int(a.historyLimitField)
	s.Normalize()
	return s
}

func (a *App) syncFields() {
	a.dirField = a.settings.DownloadDir
	a.connsField = float64(a.settings.Connections)
	a.concurrentField = float64(a.settings.MaxConcurrent)
	a.portField = float64(a.settings.APIPort)
	a.historyDaysField = float64(a.settings.HistoryDays)
	a.historyLimitField = float64(a.settings.HistoryLimit)
	a.langTag = a.settings.Language
	a.themePick = themeIndex(a.settings.Theme)
}
