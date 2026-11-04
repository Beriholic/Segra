package main

import (
	"net/http"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/internal/store"
	"github.com/beriholic/segra/locales"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()

	a := &app{view: "all", selected: -1, downloadsOpen: true, settingsOpen: true}
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
	t.Cleanup(func() { a.mgr.Close() })

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
		Selected: &a.selected,
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
