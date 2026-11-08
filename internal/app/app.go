package app

import (
	"log"
	"net/http"
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

// app is the state of Segra's main window. The view reads it on the main
// thread; downloads change it through Manager notifications.
type App struct {
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
	paused   bool

	// panel is the menu bar panel while it shows, and panelRows the number
	// of downloads it lists: a download that starts or finishes resizes it.
	panel     *mygo.Window
	panelRows int
	// trayTitle and trayTip are what the icon says already, so that the
	// second the downloads do not change says nothing to the system.
	trayTitle string
	trayTip   string

	tasks    []download.Task
	rows     []download.Task
	list     ui.ListState
	selected int
	view     string
	search   string

	// detailsCollapsed hides the task details panel; false keeps it open.
	detailsCollapsed bool

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
	langPick        string // the label the language picker shows

	// speedSamples is the throughput of the last minute, one sample a
	// second, written by sampleSpeeds and read by the speed chart.
	speedMu      sync.Mutex
	speedSamples []float64
	sampleStop   chan struct{}
	// sparkBuf, sparkSmooth and sparkPts hold what the speed chart draws,
	// reused every frame so that a chart at rest does not allocate.
	sparkBuf    []float64
	sparkSmooth []float64
	sparkPts    [][2]float32
}

// start opens the store, the download manager and the local API, then the
// window.
func (a *App) start() {
	if a.settings.Language != "" {
		mygo.App.SetLocale(a.settings.Language)
	}

	transport := netproxy.Transport()
	// Keep enough idle connections for every worker to get its own, and let
	// fresh requests skip the handshake. Surge's downloader relies on the
	// same pool shape.
	transport.MaxIdleConns = 128
	transport.MaxIdleConnsPerHost = 64
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
		// Only what the user left running: a task they paused on purpose
		// waits for them to resume it.
		a.mgr.ResumeInterrupted()
	}

	if a.settings.EnableAPI {
		a.startAPI()
	}

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
		MenuKeepsSelection: true,
	}
	a.refresh()

	a.sampleStop = make(chan struct{})
	go a.sampleSpeeds(a.sampleStop)

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:         "Segra",
		Width:         1180,
		Height:        760,
		MinWidth:      980,
		MinHeight:     620,
		TitleBarStyle: mygo.TitleBarHidden,
		StateKey:      "main-a",
		Content:       ui.View(a.viewFn),
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
func (a *App) shutdown() {
	a.persist()
	if a.sampleStop != nil {
		close(a.sampleStop)
		a.sampleStop = nil
	}
	if a.srv != nil {
		_ = a.srv.Stop()
	}
	if a.mgr != nil {
		a.mgr.Close()
	}
}

func (a *App) requestRefresh() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Update(func() { a.refresh() })
}

func (a *App) persist() {
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

func (a *App) persistAsync() {
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

func (a *App) refresh() {
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
	if a.selected < 0 && len(a.rows) > 0 && a.view != "settings" {
		a.selected = 0
	}
	// A pause sends no more progress, so nothing would repaint the window
	// and the chart would sit frozen with the curve in mid-air.
	a.settlePause()
	if mygo.App.IsReady() {
		mygo.App.SetBadgeCount(a.mgr.ActiveCount())
	}
}

// sampleSpeeds records the throughput of the running downloads once a
// second, whatever the window is doing: the chart of the last minute has to
// keep its history while the app sits in the background, and it must read
// zero at once while a download is paused, not where the frame of the pause
// happened to leave it.
func (a *App) sampleSpeeds(stop chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			tasks := a.mgr.List()
			var total float64
			for _, t := range tasks {
				if t.Status == download.StatusDownloading {
					total += float64(t.Speed)
				}
			}
			a.speedMu.Lock()
			a.speedSamples = append(a.speedSamples, total)
			if len(a.speedSamples) > sparkSlots {
				a.speedSamples = a.speedSamples[len(a.speedSamples)-sparkSlots:]
			}
			a.speedMu.Unlock()
			a.updateTrayStatus(tasks)
		}
	}
}

// settlePause repaints the window a few times after downloads pause: the
// manager sends nothing once the bytes stop, so the curve of the speed chart
// would stay where the last frame left it, halfway down. A handful of frames
// is enough for it to reach the floor; after that the chart is at rest and
// the sampler keeps its history up to date for when it shows again.
func (a *App) settlePause() {
	paused := false
	for _, t := range a.tasks {
		if t.Status == download.StatusPaused {
			paused = true
			break
		}
	}
	a.mu.Lock()
	changed := paused != a.paused
	a.paused = paused
	win := a.win
	a.mu.Unlock()
	if !changed || !paused || win == nil {
		return
	}
	go func() {
		for range 8 {
			time.Sleep(time.Second)
			a.mu.Lock()
			quitting := a.quitting
			a.mu.Unlock()
			if quitting {
				return
			}
			win.Update(func() { a.refresh() })
		}
	}()
}

func (a *App) matchesView(t download.Task) bool {
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

func (a *App) onDownloadEvent(e download.Event) {
	a.persistAsync()
	switch e.Kind {
	case download.EventCompleted:
		a.notify(locales.NotifCompleted(), e.Task.FileName, e.Task.ID)
	case download.EventFailed:
		a.notify(locales.NotifFailed(), e.Task.FileName, e.Task.ID)
	}
}

func (a *App) notify(title, body, id string) {
	if !a.settings.Notifications || !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{ID: "segra-" + id + "-" + title, Title: title, Body: body})
	n.OnClick(a.showWindow)
	if err := n.Show(); err != nil {
		log.Printf("notification: %v", err)
	}
}

func (a *App) invalidate() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}

func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Show()
	win.Focus()
}
