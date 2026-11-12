package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

// loadFinished puts one finished record in the manager, with a file on disk
// for the completed one.
func loadFinished(t *testing.T, a *App, id, name string, status download.Status, ended time.Time) string {
	t.Helper()
	path := filepath.Join(a.settings.DownloadDir, name)
	if status == download.StatusCompleted {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.mgr.Load([]download.Task{{
		ID: id, URL: "https://example.com/" + name, FileName: name,
		Dir: a.settings.DownloadDir, Size: 4, Status: status,
		CompletedAt: ended, StartedAt: ended, CreatedAt: ended,
	}})
	a.refresh()
	return path
}

// The header clears the finished records by hand, and the downloads stay:
// what the list forgets is a line, not a file.
func TestHeaderClearsFinishedHistory(t *testing.T) {
	a := newTestApp(t)
	file := loadFinished(t, a, "done", "done.iso", download.StatusCompleted, time.Now())
	loadFinished(t, a, "broke", "broke.iso", download.StatusFailed, time.Now())

	tt := ui.NewTester(a.viewFn, 1180, 760)
	tt.Frame()
	if err := tt.Click(locales.ClearHistory()); err != nil {
		t.Fatalf("the header offers no history menu: %v", err)
	}
	want := fmt.Sprintf("%s (1)", locales.ClearCompleted())
	if menu := tt.Menu(); len(menu) != 2 || menu[0] != want {
		t.Fatalf("menu %q, want the completed count first: %q", menu, want)
	}
	if err := tt.ChooseMenuItem(want); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.mgr.Get("done"); ok {
		t.Fatal("the completed record is still listed")
	}
	if _, ok := a.mgr.Get("broke"); !ok {
		t.Fatal("clearing the completed took the failed record with it")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("clearing the history deleted the download: %v", err)
	}
}

// The history keeps itself: a record past the days the settings keep goes
// when the settings change, without the user clearing anything.
func TestHistorySettingsSaveAndTidyTheList(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	a.settings.HistoryDays = 30
	a.syncFields()
	loadFinished(t, a, "old", "old.iso", download.StatusCompleted, time.Now().Add(-60*24*time.Hour))
	a.refresh()

	tt := ui.NewTester(a.viewFn, 1180, 1400)
	tt.Frame()
	for _, text := range []string{locales.History(), locales.AutoCleanHistory(), locales.KeepAtMost()} {
		if !tt.HasText(text) {
			t.Fatalf("texts %q do not offer %q", tt.Texts(), text)
		}
	}

	// What the days field holds when it changes is what the settings save.
	a.historyDaysField = 1
	a.applySettings()

	saved, err := a.store.LoadSettings(store.DefaultSettings(""))
	if err != nil {
		t.Fatal(err)
	}
	if saved.HistoryDays != 1 {
		t.Fatalf("saved days = %d, want 1", saved.HistoryDays)
	}
	if _, ok := a.mgr.Get("old"); ok {
		t.Fatal("a record older than the days the settings keep is still listed")
	}
}

// With the clean-up off, nothing is dropped however old the record is.
func TestHistoryIsKeptWholeWhenTheCleanUpIsOff(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	a.settings.HistoryAutoClean = false
	a.syncFields()
	loadFinished(t, a, "old", "old.iso", download.StatusCompleted, time.Now().Add(-600*24*time.Hour))
	a.refresh()

	a.applySettings()

	if _, ok := a.mgr.Get("old"); !ok {
		t.Fatal("the history was tidied while the clean-up was off")
	}
}
