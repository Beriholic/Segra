// Package app is Segra's desktop app: the window, the views it draws, and the
// actions they trigger. The download engine, the local API and the store live
// in their own internal packages; Run wires them to a mygo app.
package app

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"

	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

// trayIcon is the menu bar image, embedded by the main package from
// resources/ and handed to Run.
var trayIcon []byte

// Run starts Segra and blocks until the app quits.
func Run(tray []byte) {
	trayIcon = tray
	mygo.App.SetName("Segra")
	// The version is the one in mygo.json, which mygo build links into the
	// binary and the bundle, so a release is numbered in one place.

	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	a := &App{
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
	a.loginSaved = settings.OpenAtLogin
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

func (a *App) buildMenu() {
	file := &mygo.MenuItem{
		Label: locales.MenuFile(),
		Submenu: []*mygo.MenuItem{
			{Label: locales.MenuNewDownload(), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { a.openNew() }},
			{Label: locales.MenuOpenFolder(), Click: func(*mygo.MenuItem, *mygo.Window) { a.openDownloadFolder() }},
			{Label: locales.MenuExportExtension(), Click: func(*mygo.MenuItem, *mygo.Window) {
				go a.exportExtension(a.settings.APIPort, a.settings.APIToken)
			}},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		},
	}
	help := &mygo.MenuItem{
		Label: locales.MenuHelp(),
		Submenu: []*mygo.MenuItem{
			{Label: locales.MenuCheckUpdates(), Click: func(*mygo.MenuItem, *mygo.Window) { a.requestUpdateCheck(true) }},
			mygo.Separator(),
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

// createTray adds the menu bar icon, which shows how far the downloads are and
// opens the panel below it. Linux's tray icons show menus and tell nothing of
// clicks, so there the icon keeps a menu.
func (a *App) createTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "segra",
		Icon:           trayIcon,
		IconIsTemplate: true,
		ToolTip:        "Segra",
	})
	if err != nil {
		log.Printf("tray: %v", err)
		return
	}
	a.mu.Lock()
	a.tray = tray
	a.mu.Unlock()

	if runtime.GOOS == "linux" {
		tray.SetMenu(a.trayMenu())
		return
	}
	tray.OnClick(a.togglePanel)
	tray.OnRightClick(func() {
		a.closePanel()
		// The menu is the icon's only for this popup, so that a click on the
		// icon keeps toggling the panel.
		tray.SetMenu(a.trayMenu())
		defer tray.SetMenu(nil)
		tray.PopUpMenu()
	})
	a.updateTrayStatus(a.mgr.List())
}

// trayMenu is the icon's menu: the actions of the panel, for a right click.
func (a *App) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		mygo.Separator(),
		{Label: locales.TrayPauseAll(), Click: func(*mygo.MenuItem, *mygo.Window) { a.mgr.PauseAll() }},
		{Label: locales.TrayResumeAll(), Click: func(*mygo.MenuItem, *mygo.Window) { a.mgr.ResumeAll() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit, Label: locales.TrayQuit()},
	})
}
