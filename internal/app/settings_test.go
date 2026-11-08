package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

// The settings have no Save button: the folder saves when its field is
// done with, so that typing a path does not write down every half of one.
func TestSettingsFolderSavesWhenItIsDone(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	dir := filepath.Join(t.TempDir(), "downloads")
	a.dirField = dir
	a.fieldDirty = true
	a.commitField()

	saved, err := a.store.LoadSettings(store.DefaultSettings(""))
	if err != nil {
		t.Fatal(err)
	}
	if saved.DownloadDir != dir {
		t.Fatalf("saved download dir = %q, want %q", saved.DownloadDir, dir)
	}
	if a.settings.DownloadDir != dir {
		t.Fatalf("running download dir = %q, want %q", a.settings.DownloadDir, dir)
	}
	if a.fieldDirty {
		t.Fatal("the field still waits to save after it was saved")
	}
}

// A switch is a setting whole, so what it flips is written down at once.
func TestSettingsSwitchSavesAsItFlips(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	a.settings.Notifications = false

	// The view binds the switch to a.settings, and the switch saves the
	// settings as it flips, the way flipping one here does.
	a.applySettings()

	saved, err := a.store.LoadSettings(store.DefaultSettings(""))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Notifications {
		t.Fatal("saved settings still show notifications on")
	}
}

func TestSettingsSayTheySaveByThemselves(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	tt := ui.NewTester(a.viewFn, 920, 1200)

	if _, ok := tt.Find("Save"); ok {
		t.Fatalf("texts %q still offer a Save button", tt.Texts())
	}
	if !tt.HasText(locales.AutoSave()) {
		t.Fatalf("texts %q do not say the settings save by themselves", tt.Texts())
	}
}
