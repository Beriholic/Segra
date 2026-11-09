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

// The appearance is a setting like the others: it is written down as it is
// picked, and the frame after it draws with it.
func TestAppearanceSavesAsItIsPicked(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	a.setTheme(store.ThemeDark)

	saved, err := a.store.LoadSettings(store.DefaultSettings(""))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Theme != store.ThemeDark {
		t.Fatalf("saved theme = %q, want %q", saved.Theme, store.ThemeDark)
	}
	tt := ui.NewTester(a.viewFn, 1180, 760)
	tt.Frame()
	if !a.pal.dark {
		t.Fatal("the frame was built with the light palette")
	}
	if r, _, _, _ := tt.Image().At(600, 640).RGBA(); r > 0x8000 {
		t.Fatalf("the page is not dark: red = %#04x", r>>8)
	}
}

// Following the desktop is the default, and it is the desktop's appearance
// that decides the palette, not a choice of the user's.
func TestAppearanceFollowsTheDesktop(t *testing.T) {
	a := newTestApp(t)
	if a.settings.Theme != store.ThemeSystem {
		t.Fatalf("a first run chose %q, want the system's", a.settings.Theme)
	}
	tt := ui.NewTester(a.viewFn, 1180, 760)
	tt.Frame()
	if a.pal.dark {
		t.Fatal("the frame is dark while the desktop is light")
	}

	// The desktop turns dark under a window that follows it.
	tt.SetDark(true)
	tt.Frame()
	if !a.pal.dark {
		t.Fatal("the frame is light while the desktop is dark")
	}
	if r, _, _, _ := tt.Image().At(600, 640).RGBA(); r > 0x8000 {
		t.Fatalf("the page is not dark: red = %#04x", r>>8)
	}
}

// The settings offer the three appearances, and the picker chooses one.
func TestSettingsChooseAnAppearance(t *testing.T) {
	a := newTestApp(t)
	a.view = "settings"
	tt := ui.NewTester(a.viewFn, 1180, 1200)
	for _, label := range []string{locales.ThemeSystem(), locales.ThemeLight(), locales.ThemeDark()} {
		if !tt.HasText(label) {
			t.Fatalf("texts %q do not offer %q", tt.Texts(), label)
		}
	}
	if err := tt.Click(locales.ThemeDark()); err != nil {
		t.Fatal(err)
	}
	if a.settings.Theme != store.ThemeDark {
		t.Fatalf("theme = %q after choosing dark, want %q", a.settings.Theme, store.ThemeDark)
	}
}
