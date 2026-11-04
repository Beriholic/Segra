package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"
	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/internal/server"
	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

//go:embed resources/icon.png
var iconPNG []byte

//go:embed resources/tray.png
var trayPNG []byte

// app is the state of Segra's main window. The view reads it on the main
// thread; downloads change it through Manager notifications.
type app struct {
	store *store.Store
	mgr   *download.Manager
	srv   *server.Server

	settings store.Settings
	apiPort  int
	apiErr   string

	mu       sync.Mutex
	win      *mygo.Window
	tray     *mygo.Tray
	quitting bool

	tasks    []download.Task
	rows     []download.Task
	list     ui.ListState
	selected int
	view     string
	search   string

	downloadsOpen bool
	settingsOpen  bool

	newOpen  bool
	newLink  string
	newDir   string
	newConns float64
	newErr   string

	dirField        string
	connsField      float64
	concurrentField float64
	portField       float64
	langTag         string
}

func main() {
	mygo.App.SetName("Segra")
	mygo.App.SetVersion("0.1.0")

	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	a := &app{
		view:          "all",
		selected:      -1,
		downloadsOpen: true,
		settingsOpen:  true,
	}

	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		dataDir = filepath.Join(os.TempDir(), "Segra")
	}
	st, err := store.Open(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	a.store = st

	downloads, _ := mygo.App.Path(mygo.PathDownloads)
	settings, err := st.LoadSettings(store.DefaultSettings(downloads))
	if err != nil {
		log.Printf("settings: %v", err)
	}
	if settings.APIToken == "" {
		settings.APIToken = newToken()
		if err := st.SaveSettings(settings); err != nil {
			log.Printf("save settings: %v", err)
		}
	}
	a.settings = settings
	a.syncFields()

	mygo.App.OnSecondInstance(func(args []string, workingDir string) { a.showWindow() })
	mygo.App.OnOpenURL(a.openDeepLink)
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" && !a.settings.CloseToTray {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.showWindow()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.shutdown()
	})

	mygo.App.WhenReady(a.start)
	mygo.App.Run()
}

// start opens the store, the download manager and the local API, then the
// window.
func (a *app) start() {
	if a.settings.Language != "" {
		mygo.App.SetLocale(a.settings.Language)
	}

	transport := netproxy.Transport()
	// Keep enough idle connections for every worker to get its own, and let
	// fresh requests skip the handshake. Surge's downloader relies on the
	// same pool shape.
	transport.MaxIdleConns = 128
	transport.MaxIdleConnsPerHost = 64
	transport.ForceAttemptHTTP2 = true
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	client := &http.Client{Transport: transport}
	a.mgr = download.New(client, download.Config{
		Dir:           a.settings.DownloadDir,
		Connections:   a.settings.Connections,
		MaxConcurrent: a.settings.MaxConcurrent,
	}, a.requestRefresh)
	a.mgr.SetOnEvent(a.onDownloadEvent)

	if tasks, err := a.store.LoadTasks(); err != nil {
		log.Printf("load tasks: %v", err)
	} else {
		for i := range tasks {
			// Tasks from 0.1.0's old default of 8 connections pick up the
			// new default so they finish at the expected speed.
			if tasks[i].Connections == 8 && a.settings.Connections > 8 {
				tasks[i].Connections = a.settings.Connections
			}
		}
		a.mgr.Load(tasks)
	}
	if a.settings.AutoResume {
		a.mgr.ResumeAll()
	}

	a.startAPI()

	a.list = ui.ListState{
		Key: func(i int) any {
			if i >= 0 && i < len(a.rows) {
				return a.rows[i].ID
			}
			return i
		},
		Label: func(i int) string {
			if i >= 0 && i < len(a.rows) {
				return a.rows[i].FileName
			}
			return ""
		},
		Selected:           &a.selected,
		MenuKeepsSelection: true,
	}
	a.refresh()

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     "Segra",
		Width:     920,
		Height:    660,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(a.viewFn),
	})
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()

	win.OnClose(func(e *mygo.CloseEvent) {
		if a.settings.CloseToTray && !a.quitting {
			e.PreventDefault()
			win.Hide()
		}
	})

	a.buildMenu()
	a.createTray()
	mygo.App.OnLocaleChanged(func(string) {
		a.buildMenu()
		win.SetTitle("Segra")
	})
}

// shutdown persists state and stops the background workers.
func (a *app) shutdown() {
	a.persist()
	if a.srv != nil {
		_ = a.srv.Stop()
	}
	if a.mgr != nil {
		a.mgr.Close()
	}
}

func (a *app) startAPI() {
	a.srv = server.New(a.mgr, a.settings.APIToken, a.settings.APIPort)
	port, err := a.srv.Start()
	if err != nil {
		a.apiErr = err.Error()
		log.Printf("api: %v", err)
		return
	}
	a.apiPort = port
	a.apiErr = ""
	log.Printf("api listening on 127.0.0.1:%d", port)
}

func (a *app) restartAPI() {
	if a.srv != nil {
		_ = a.srv.Stop()
	}
	a.startAPI()
}

func (a *app) requestRefresh() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Update(func() { a.refresh() })
}

func (a *app) persist() {
	if a.mgr == nil {
		return
	}
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
	if err := a.store.SaveTasks(a.mgr.List()); err != nil {
		log.Printf("save tasks: %v", err)
	}
}

func (a *app) persistAsync() {
	if a.mgr == nil {
		return
	}
	tasks := a.mgr.List()
	s := a.settings
	go func() {
		if err := a.store.SaveTasks(tasks); err != nil {
			log.Printf("save tasks: %v", err)
		}
		if err := a.store.SaveSettings(s); err != nil {
			log.Printf("save settings: %v", err)
		}
	}()
}

func (a *app) refresh() {
	a.tasks = a.mgr.List()
	query := strings.ToLower(strings.TrimSpace(a.search))
	a.rows = a.rows[:0]
	for _, t := range a.tasks {
		if !a.matchesView(t) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(t.FileName+" "+t.URL), query) {
			continue
		}
		a.rows = append(a.rows, t)
	}
	if a.selected >= len(a.rows) {
		a.selected = len(a.rows) - 1
	}
	if a.selected < -1 {
		a.selected = -1
	}
	if mygo.App.IsReady() {
		mygo.App.SetBadgeCount(a.mgr.ActiveCount())
	}
}

func (a *app) matchesView(t download.Task) bool {
	switch a.view {
	case "active":
		return t.Status.Active()
	case "completed":
		return t.Status == download.StatusCompleted
	case "failed":
		return t.Status == download.StatusFailed
	default:
		return true
	}
}

func (a *app) onDownloadEvent(e download.Event) {
	a.persistAsync()
	switch e.Kind {
	case download.EventCompleted:
		a.notify(locales.NotifCompleted(), e.Task.FileName, e.Task.ID)
	case download.EventFailed:
		a.notify(locales.NotifFailed(), e.Task.FileName, e.Task.ID)
	}
}

func (a *app) notify(title, body, id string) {
	if !a.settings.Notifications || !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{ID: "segra-" + id + "-" + title, Title: title, Body: body})
	n.OnClick(a.showWindow)
	if err := n.Show(); err != nil {
		log.Printf("notification: %v", err)
	}
}

func (a *app) invalidate() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}

func (a *app) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Show()
	win.Focus()
}

func (a *app) openDeepLink(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	link := u.Query().Get("url")
	if link == "" && u.Host != "" && u.Host != "download" {
		link = "https://" + u.Host + u.Path
	}
	if link == "" {
		return
	}
	a.openNew()
	a.newLink = link
	a.showWindow()
	a.invalidate()
}

func (a *app) openNew() {
	dir := a.settings.DownloadDir
	if dir == "" || dir == "." {
		if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
			dir = downloads
		}
	}
	a.newOpen = true
	a.newLink = ""
	a.newDir = dir
	a.newConns = float64(a.settings.Connections)
	a.newErr = ""
	a.invalidate()
}

func (a *app) submitNew() {
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

func (a *app) browseNewDir() {
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

func (a *app) browseSettingsDir() {
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

func (a *app) toggle(t download.Task) {
	switch t.Status {
	case download.StatusDownloading, download.StatusQueued:
		a.mgr.Pause(t.ID)
	case download.StatusPaused, download.StatusFailed:
		a.mgr.Resume(t.ID)
	}
}

func (a *app) remove(t download.Task, deleteFiles bool) {
	a.mgr.Remove(t.ID, deleteFiles)
	a.persistAsync()
}

func (a *app) reveal(t download.Task) {
	path := t.Path()
	if _, err := os.Stat(path); err == nil {
		mygo.Shell.ShowItemInFolder(path)
		return
	}
	mygo.Shell.OpenPath(t.Dir)
}

func (a *app) openDownloadFolder() {
	dir := a.settings.DownloadDir
	if dir == "" || dir == "." {
		if downloads, err := mygo.App.Path(mygo.PathDownloads); err == nil {
			dir = downloads
		}
	}
	mygo.Shell.OpenPath(dir)
}

func (a *app) setLanguage(tag string) {
	a.settings.Language = tag
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
	mygo.App.SetLocale(tag)
	a.buildMenu()
}

func (a *app) applySettings(c *ui.Context) {
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
	loginChanged := s.OpenAtLogin != a.settings.OpenAtLogin
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
	if loginChanged {
		if err := mygo.App.SetOpenAtLogin(s.OpenAtLogin); err != nil {
			log.Printf("open at login: %v", err)
			c.Toast(err.Error())
		}
	}
	if portChanged {
		a.restartAPI()
		if a.apiErr != "" {
			c.Toast(a.apiErr)
			return
		}
		c.Toast(locales.PortChanged())
		return
	}
	c.Toast(locales.Saved())
}

func (a *app) syncFields() {
	a.dirField = a.settings.DownloadDir
	a.connsField = float64(a.settings.Connections)
	a.concurrentField = float64(a.settings.MaxConcurrent)
	a.portField = float64(a.settings.APIPort)
	a.langTag = a.settings.Language
}

func (a *app) buildMenu() {
	file := &mygo.MenuItem{
		Label: locales.MenuFile(),
		Submenu: []*mygo.MenuItem{
			{Label: locales.MenuNewDownload(), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { a.openNew() }},
			{Label: locales.MenuOpenFolder(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openDownloadFolder() }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		},
	}
	help := &mygo.MenuItem{
		Label: locales.MenuHelp(),
		Submenu: []*mygo.MenuItem{
			{Label: locales.MenuAbout(), Click: func(*mygo.MenuItem, *mygo.Window) {
				mygo.App.ShowAboutPanel(mygo.AboutPanelOptions{ApplicationName: "Segra", ApplicationVersion: mygo.App.Version()})
			}},
		},
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		file,
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
		help,
	})
	mygo.App.SetMenu(menu)

	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil && runtime.GOOS != "darwin" {
		win.SetMenu(menu)
	}
}

func (a *app) createTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "segra",
		Icon:           trayPNG,
		IconIsTemplate: true,
		ToolTip:        "Segra",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			mygo.Separator(),
			{Label: locales.TrayPauseAll(), Click: func(*mygo.MenuItem, *mygo.Window) { a.mgr.PauseAll() }},
			{Label: locales.TrayResumeAll(), Click: func(*mygo.MenuItem, *mygo.Window) { a.mgr.ResumeAll() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit, Label: locales.TrayQuit()},
		}),
	})
	if err != nil {
		log.Printf("tray: %v", err)
		return
	}
	a.tray = tray
	tray.OnClick(a.showWindow)
}

func newToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
