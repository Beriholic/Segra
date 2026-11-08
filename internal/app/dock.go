package app

import (
	"runtime"

	"github.com/egoist/mygo"
)

// The Dock icon can follow the window on macOS: there while the window is
// open, gone once it closes, so that a download finishing in the background
// does not keep a Dock slot. Segra is a menu bar app first; the window is the
// only thing the Dock is good for here, and a closed window cannot be reopened
// from it.

// dockHidden reports whether macOS shows no Dock icon for Segra.
func (a *App) dockHidden() bool {
	return mygo.App.ActivationPolicy() == mygo.ActivationPolicyAccessory
}

// setDockHidden hides or shows the Dock icon, doing nothing on the platforms
// that have none, and nothing when it is already that way.
func (a *App) setDockHidden(hidden bool) {
	if runtime.GOOS != "darwin" || hidden == a.dockHidden() {
		return
	}
	if hidden {
		mygo.App.Dock.Hide()
		return
	}
	mygo.App.Dock.Show()
}

// syncDock makes the Dock icon match the window and the setting: shown while
// the window is open, hidden once it closes if the user asked for that. The
// policy redraws the Dock, so it is asked for on the way in and out of the
// window rather than every frame.
func (a *App) syncDock() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	open := win != nil && win.IsVisible()
	a.setDockHidden(!open && a.settings.HideDock)
}
