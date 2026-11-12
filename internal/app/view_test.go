package app

import (
	"net/http"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()

	a := &App{view: "all", selected: -1, downloadsOpen: true, settingsOpen: true}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.store = st
	a.settings = store.DefaultSettings(dir)
	a.settings.DownloadDir = dir
	a.settings.APIToken = "test-token"
	a.syncFields()

	a.mgr = download.New(&http.Client{}, download.Config{
		Dir:           dir,
		Connections:   2,
		MaxConcurrent: 2,
	}, func() {})
	// The workers go first, then the saves they asked for: a write still in
	// flight when the temp dir goes fails its rename and fails the test.
	t.Cleanup(func() {
		a.mgr.Close()
		a.waitSaves()
	})

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
	}
	return a
}

func TestViewShowsTasks(t *testing.T) {
	a := newTestApp(t)
	a.mgr.Load([]download.Task{{
		ID:         "one",
		URL:        "https://example.com/ubuntu.iso",
		FileName:   "ubuntu.iso",
		Dir:        a.settings.DownloadDir,
		Size:       4096,
		Downloaded: 1024,
		Status:     download.StatusCompleted,
	}})

	tt := ui.NewTester(a.viewFn, 920, 660)
	if !tt.HasText("ubuntu.iso") {
		t.Fatalf("texts %q do not show the file", tt.Texts())
	}
	if !tt.HasText(locales.CompletedStatus()) {
		t.Fatalf("texts %q do not show the status", tt.Texts())
	}
}

func TestNewDownloadDialog(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.viewFn, 920, 660)

	if err := tt.Click(locales.NewDownload()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.NewDownloadTitle()) {
		t.Fatalf("texts %q do not show the dialog", tt.Texts())
	}
	if !a.newOpen {
		t.Fatal("dialog is not open")
	}
}

func TestEmptyState(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.viewFn, 920, 660)
	if !tt.HasText(locales.NoTasks()) {
		t.Fatalf("texts %q do not show the empty state", tt.Texts())
	}
}

func TestClipboardLink(t *testing.T) {
	cases := map[string]string{
		"https://example.com/file.zip":  "https://example.com/file.zip",
		"  https://example.com/a?b=1  ": "https://example.com/a?b=1",
		"http://example.com/file.zip":   "http://example.com/file.zip",
		"not a link":                    "",
		"hello":                         "",
		"ftp://example.com/file.zip":    "",
		"https://":                      "",
		"example.com/file.zip":          "",
		"https://example.com/a b":       "",
		"https://example.com/a\nsecond": "",
	}
	for in, want := range cases {
		if got := clipboardLink(in); got != want {
			t.Errorf("clipboardLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSettingsOnMacOfferOnlyTheDockChoice(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS has a Dock")
	}
	a := newTestApp(t)
	a.view = "settings"
	a.settings.CloseToTray = false
	tt := ui.NewTester(a.viewFn, 920, 1200)

	if !tt.HasText(locales.HideDock()) {
		t.Fatalf("texts %q do not offer the Dock setting", tt.Texts())
	}
	// A menu bar app keeps running when its window closes, so offering to
	// keep running would ask a question with one answer.
	if tt.HasText(locales.CloseToTray()) {
		t.Fatalf("texts %q offer the setting that macOS answers itself", tt.Texts())
	}
}
