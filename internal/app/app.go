package app

import (
	"log"
	"net/http"
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
	"github.com/beriholic/segra/internal/update"
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
	// badge is the count the dock shows already, so that a frame that
	// changes nothing does not tell the system again.
	badge int
	// toast is a message a worker left for the next frame to show, as the
	// outcome of a dialog that ran off the main thread.
	toast string
	// loginSaved is what the system was last told about starting at login.
	// The switch writes a.settings as it flips, so nothing else remembers
	// the value the system has.
	loginSaved bool

	// updateMu guards the result of the last release check, which a worker
	// writes and the view reads. updateClient is what that worker asks
	// GitHub with, and nil in tests, which never start a check.
	updateMu     sync.Mutex
	updating     bool
	updateResult *update.Result
	updateErr    string
	updateClient *http.Client

	// updateOpen shows the release dialog, and updateRun replays its
	// entrance when it opens again. Both are the view's, on the main
	// thread.
	updateOpen bool
	updateRun  uint64
	// notifiedUpdate is the release the user was told about already, so
	// that the daily check does not repeat the same notification.
	notifiedUpdate string
	// updateStop ends the daily check when the app quits.
	updateStop chan struct{}

	// seenAt is when each download first showed, so that its row slides in
	// once rather than every time the list scrolls it back into view.
	seenAt map[string]time.Time
	// finishedAt is when each download landed, for the moment its row shows
	// it. The manager's events write it off the main thread.
	finishedAt map[string]time.Time

	// newRun counts the times the New Download sheet opened, so that its
	// entrance plays again rather than only once.
	newRun uint64

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

	// pal is the palette the frame draws with, picked from the appearance as
	// each view builds, and themePick the segment the picker shows.
	pal       palette
	themePick int

	dirField        string
	connsField      float64
	concurrentField float64
	portField       float64
	// fieldDirty holds a folder typed into the settings that has not become
	// a setting yet: it saves once the field is left, not on every letter.
	fieldDirty bool
	langTag    string
	langPick   string // the label the language picker shows

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
	// sparkPrev, sparkShown and sparkAt morph the curve from the samples it
	// drew to the ones it has now, and sparkMorph holds the values on the
	// way, reused every frame.
	sparkPrev  []float64
	sparkShown []float64
	sparkAt    time.Time
	sparkMorph []float64

	// insHeat is the chunk map of the task the inspector shows, and
	// panelHeat those of the downloads the menu bar panel lists: each
	// tweens its cells from the states it drew to the ones they have now.
	insHeat    heatTween
	panelHeats map[string]*heatTween
}

// start opens the store, the download manager and the local API, then the
// window.
func (a *App) start() {
	if a.settings.Language != "" {
		mygo.App.SetLocale(a.settings.Language)
	}

	// The login item belongs to the system, not to the settings file: it
	// outlives the app, and the user can remove it in System Settings. The
	// setting is applied again on every launch, so one made before this
	// build, or removed elsewhere, is made again.
	if a.settings.OpenAtLogin {
		if err := mygo.App.SetOpenAtLogin(true); err != nil {
			// Let the next save of the settings try again.
			a.loginSaved = false
			log.Printf("open at login: %v", err)
		}
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
	// Release checks share the proxy-aware transport but not the
	// downloader's patience: a check that hangs would leave the settings
	// saying it is still looking.
	a.updateClient = &http.Client{Transport: transport, Timeout: updateTimeout}
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
	a.updateStop = make(chan struct{})
	go a.watchUpdates(a.updateStop)

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
		if a.quitting {
			return
		}
		// Closing the window leaves Segra running in the menu bar. macOS does
		// that for every app of its kind, so the choice of the Dock icon is
		// the only one left; elsewhere the window would close for good unless
		// the user asked for the tray.
		if runtime.GOOS == "darwin" || a.settings.CloseToTray {
			e.PreventDefault()
			win.Hide()
			a.syncDock()
		}
	})

	a.buildMenu()
	a.createTray()
	mygo.App.OnLocaleChanged(func(string) {
		a.buildMenu()
		win.SetTitle("Segra")
	})

	// The window is up by now, so a release found at launch has somewhere
	// to say so. The check runs off the main thread.
	a.maybeCheckUpdates()
}

// shutdown persists state and stops the background workers.
func (a *App) shutdown() {
	a.persist()
	if a.sampleStop != nil {
		close(a.sampleStop)
		a.sampleStop = nil
	}
	if a.updateStop != nil {
		close(a.updateStop)
		a.updateStop = nil
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
	// Whatever the settings view holds is what the user sees, even when
	// the window closes with the caret still in a field.
	a.settings = a.captureSettings()
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
	a.noteTasks(a.tasks, time.Now())
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
		// The dock is told only when the count it shows changes: a frame
		// that animates the window would otherwise call the system sixty
		// times a second for nothing.
		if n := a.mgr.ActiveCount(); n != a.badge {
			a.badge = n
			mygo.App.SetBadgeCount(n)
		}
	}
}

// noteTasks remembers when each download first showed, and forgets the ones
// gone: the list slides a row in for a new download, not for one that
// scrolled back into view.
func (a *App) noteTasks(tasks []download.Task, now time.Time) {
	if a.seenAt == nil {
		a.seenAt = make(map[string]time.Time, len(tasks))
	}
	for _, t := range tasks {
		if _, ok := a.seenAt[t.ID]; !ok {
			a.seenAt[t.ID] = now
		}
	}
	if len(a.seenAt) > 2*len(tasks)+8 {
		live := make(map[string]time.Time, len(tasks))
		for _, t := range tasks {
			if at, ok := a.seenAt[t.ID]; ok {
				live[t.ID] = at
			}
		}
		a.seenAt = live
	}
}

// fresh reports whether the download showed up within d.
func (a *App) fresh(id string, d time.Duration) bool {
	at, ok := a.seenAt[id]
	return ok && time.Since(at) < d
}

// markFinished remembers that a download landed, for its row to show it.
func (a *App) markFinished(id string) {
	a.mu.Lock()
	if a.finishedAt == nil {
		a.finishedAt = map[string]time.Time{}
	}
	if len(a.finishedAt) > 64 {
		for k, at := range a.finishedAt {
			if time.Since(at) > finishFor {
				delete(a.finishedAt, k)
			}
		}
	}
	a.finishedAt[id] = time.Now()
	a.mu.Unlock()
}

// finishedAgo is how long ago the download landed, and whether it did.
func (a *App) finishedAgo(id string) (time.Duration, bool) {
	a.mu.Lock()
	at, ok := a.finishedAt[id]
	a.mu.Unlock()
	if !ok {
		return 0, false
	}
	return time.Since(at), true
}

// say leaves message for the next frame to show as a toast. Work that runs
// off the main thread, as a save dialog does, has no frame to hand, and a
// toast counts its seconds from the frame that adds it: leaving the words
// is all it can do, and the view says them with a clock of its own.
func (a *App) say(message string) {
	a.mu.Lock()
	a.toast = message
	a.mu.Unlock()
	a.invalidate()
}

// takeToast takes the message a worker left, if any.
func (a *App) takeToast() string {
	a.mu.Lock()
	message := a.toast
	a.toast = ""
	a.mu.Unlock()
	return message
}

// flash is how much of the moment a landed download shows: 1 as it lands,
// 0 once the moment is over.
func (a *App) flash(id string) float32 {
	ago, ok := a.finishedAgo(id)
	if !ok || ago > finishFor {
		return 0
	}
	return 1 - float32(ago)/float32(finishFor)
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
		a.markFinished(e.Task.ID)
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

// invalidatePanel repaints the menu bar panel, which shows the same
// appearance as the window.
func (a *App) invalidatePanel() {
	a.mu.Lock()
	panel := a.panel
	a.mu.Unlock()
	if panel != nil {
		panel.Invalidate()
	}
}

func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	// The Dock icon comes back before the window does, so that the window
	// opens as one of a regular app, in front and with its menu bar.
	a.setDockHidden(false)
	win.Show()
	win.Focus()
}
