package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/egoist/mygo"

	"github.com/beriholic/segra/internal/update"
	"github.com/beriholic/segra/locales"
)

const (
	// updateTimeout bounds one check, so a stalled connection cannot leave
	// the settings saying it is still looking.
	updateTimeout = 20 * time.Second
	// updateInterval is how long an automatic check waits before asking
	// GitHub again. GitHub allows 60 requests an hour for an address, and
	// one check a day is plenty for a release.
	updateInterval = 24 * time.Hour
)

// updateSnapshot is what the view needs to know about the last check.
type updateSnapshot struct {
	Checking bool
	Result   *update.Result
	Err      string
}

// maybeCheckUpdates asks for a newer release, unless the user turned that
// off or a check found nothing newer within a day. It runs on the main
// thread, at launch and once a day after that: Segra lives in the menu
// bar, and a window that never closes would otherwise ask only once.
func (a *App) maybeCheckUpdates() {
	if !a.settings.CheckUpdates {
		return
	}
	if last := a.settings.UpdateCheckedAt; last > 0 && time.Since(time.Unix(last, 0)) < updateInterval {
		return
	}
	a.requestUpdateCheck(false)
}

// watchUpdates asks once a day while the app runs, so that a long-lived
// menu bar app checks as often as one that is opened each morning.
func (a *App) watchUpdates(stop chan struct{}) {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			a.mu.Lock()
			win := a.win
			a.mu.Unlock()
			if win == nil {
				continue
			}
			// Reading the settings and starting the check belong to the
			// main thread, as the launch check does.
			win.Update(func() { a.maybeCheckUpdates() })
		}
	}
}

// requestUpdateCheck starts one check. manual checks happen because the
// user asked, and say what they found; the others stay quiet unless there
// is a release to announce. It runs on the main thread.
func (a *App) requestUpdateCheck(manual bool) {
	a.updateMu.Lock()
	if a.updating {
		a.updateMu.Unlock()
		return
	}
	a.updating = true
	a.updateErr = ""
	a.updateMu.Unlock()
	a.invalidate()
	go a.runUpdateCheck(manual)
}

// runUpdateCheck asks GitHub off the main thread and hands the answer back
// to it, where the notification and the dialog belong.
func (a *App) runUpdateCheck(manual bool) {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	client := a.updateClient
	if client == nil {
		client = &http.Client{Timeout: updateTimeout}
	}
	result, err := update.Check(ctx, client, mygo.App.Version())

	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		// Nothing to show the answer in; the next check starts over.
		a.updateMu.Lock()
		a.updating = false
		a.updateErr = ""
		if err != nil {
			a.updateErr = err.Error()
		} else {
			a.updateResult = &result
		}
		a.updateMu.Unlock()
		return
	}
	win.Update(func() { a.finishUpdateCheck(result, err, manual) })
}

// finishUpdateCheck publishes the answer and, when the user asked, says it.
// It runs on the main thread.
func (a *App) finishUpdateCheck(result update.Result, err error, manual bool) {
	a.updateMu.Lock()
	a.updating = false
	if err != nil {
		a.updateErr = err.Error()
		a.updateResult = nil
	} else {
		a.updateErr = ""
		a.updateResult = &result
	}
	a.updateMu.Unlock()

	if err == nil {
		// An available release is not remembered as checked: the next
		// launch asks again until the app is updated, where a check that
		// found nothing newer waits a day.
		if result.Newer {
			a.settings.UpdateCheckedAt = 0
		} else {
			a.settings.UpdateCheckedAt = time.Now().Unix()
		}
		if a.store != nil {
			if saveErr := a.store.SaveSettings(a.settings); saveErr != nil {
				log.Printf("save settings: %v", saveErr)
			}
		}
	}

	if err != nil {
		log.Printf("check updates: %v", err)
		if manual {
			a.say(locales.UpdateCheckFailed())
		}
		a.invalidate()
		return
	}

	switch {
	case !result.Comparable:
		a.updateOpen = false
		if manual {
			a.say(locales.DevBuild())
		}
	case result.Newer:
		if manual {
			a.view = "settings"
			a.openUpdateDialog()
			a.notifiedUpdate = result.Release.Tag
		} else if a.notifiedUpdate != result.Release.Tag {
			a.notifyUpdate(result.Release)
			a.notifiedUpdate = result.Release.Tag
		}
	default:
		a.updateOpen = false
		if manual {
			a.say(locales.UpToDate())
		}
	}
	a.invalidate()
}

// snapshotUpdate reads the state of the last check for the view.
func (a *App) snapshotUpdate() updateSnapshot {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	snapshot := updateSnapshot{Checking: a.updating, Err: a.updateErr}
	if a.updateResult != nil {
		result := *a.updateResult
		snapshot.Result = &result
	}
	return snapshot
}

// updateAvailable reports whether the last check found a newer release.
func (a *App) updateAvailable() bool {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateResult != nil && a.updateResult.Comparable && a.updateResult.Newer
}

// notifyUpdate tells the user that a release is waiting, without taking
// over the window. Clicking the notification opens the release dialog.
func (a *App) notifyUpdate(release update.Release) {
	if !a.settings.Notifications || !mygo.NotificationsSupported() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "segra-update-" + release.Tag,
		Title: locales.NotifUpdate(),
		Body:  fmt.Sprintf(locales.NotifUpdateBody(), versionLabel(release.Tag)),
	})
	n.OnClick(a.showUpdate)
	if err := n.Show(); err != nil {
		log.Printf("notification: %v", err)
	}
}

// showUpdate brings the window forward on the update it found.
func (a *App) showUpdate() {
	a.showWindow()
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Update(func() {
		a.view = "settings"
		a.openUpdateDialog()
		a.refresh()
	})
}

// openUpdateDialog shows the release notes of the update. It runs on the
// main thread.
func (a *App) openUpdateDialog() {
	if !a.updateAvailable() {
		return
	}
	a.updateOpen = true
	a.updateRun++
	a.invalidate()
}

// downloadUpdate opens the release file for this platform, or the release
// page when the release has no file for it. The browser downloads it; the
// app does not install an update over itself.
func (a *App) downloadUpdate() {
	snapshot := a.snapshotUpdate()
	if snapshot.Result == nil {
		return
	}
	release := snapshot.Result.Release
	target := release.URL
	if asset, ok := release.AssetFor(runtime.GOOS); ok && asset.URL != "" {
		target = asset.URL
	}
	a.updateOpen = false
	a.openExternal(target)
}

// openUpdatePage opens the release on GitHub, where the notes and every
// file of it are.
func (a *App) openUpdatePage() {
	snapshot := a.snapshotUpdate()
	if snapshot.Result == nil {
		return
	}
	a.openExternal(snapshot.Result.Release.URL)
}

// openExternal hands an http(s) link to the browser, refusing anything
// else that a release could name.
func (a *App) openExternal(raw string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		a.say(locales.OpenLinkFailed())
		return
	}
	if err := mygo.Shell.OpenExternal(raw); err != nil {
		log.Printf("open %s: %v", raw, err)
		a.say(locales.OpenLinkFailed())
	}
}

// versionLabel writes a version the way the release and the app show it.
func versionLabel(version string) string {
	if version == "" {
		return locales.DevBuild()
	}
	if version[0] != 'v' && version[0] != 'V' {
		return "v" + version
	}
	return version
}
