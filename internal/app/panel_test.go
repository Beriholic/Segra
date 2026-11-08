package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

func TestPanelListsUnfinishedDownloads(t *testing.T) {
	tasks := []download.Task{
		{ID: "a", FileName: "a.iso", Status: download.StatusDownloading},
		{ID: "b", FileName: "b.iso", Status: download.StatusCompleted},
		{ID: "c", FileName: "c.iso", Status: download.StatusFailed},
		{ID: "d", FileName: "d.iso", Status: download.StatusPaused},
		{ID: "e", FileName: "e.iso", Status: download.StatusQueued},
		{ID: "f", FileName: "f.iso", Status: download.StatusDownloading},
	}
	rows := panelTasks(tasks)
	if len(rows) != panelMaxRows {
		t.Fatalf("rows = %d, want %d", len(rows), panelMaxRows)
	}
	want := []string{"a", "d", "e"}
	for i, id := range want {
		if rows[i].ID != id {
			t.Errorf("row %d = %q, want %q", i, rows[i].ID, id)
		}
	}
}

func TestPanelCountsEveryDownload(t *testing.T) {
	var tasks []download.Task
	for i := 0; i < 5; i++ {
		tasks = append(tasks, download.Task{
			ID:       fmt.Sprint(i),
			FileName: fmt.Sprintf("file-%d.iso", i),
			Status:   download.StatusDownloading,
		})
	}
	a := newTestApp(t)
	a.mgr.Load(tasks) // which marks the downloads it left running as paused

	if got := panelTotal(a.mgr.List()); got != 5 {
		t.Errorf("panelTotal = %d, want 5", got)
	}
	if got := len(panelTasks(a.mgr.List())); got != panelMaxRows {
		t.Errorf("rows = %d, want %d", got, panelMaxRows)
	}
	tt := ui.NewTester(a.panelView, panelWidth, panelHeight(panelMaxRows))
	if want := fmt.Sprintf("%s · 5", locales.Paused()); !tt.HasText(want) {
		t.Fatalf("texts %q do not show %q", tt.Texts(), want)
	}
}

func TestPanelHeightFollowsRows(t *testing.T) {
	if panelHeight(0) >= panelHeight(1) {
		t.Fatalf("height(0) = %d, height(1) = %d", panelHeight(0), panelHeight(1))
	}
	if panelHeight(2)-panelHeight(1) != panelRowHeight()+panelTaskGap {
		t.Errorf("a row adds %d, want %d", panelHeight(2)-panelHeight(1), panelRowHeight()+panelTaskGap)
	}
	if panelHeight(panelMaxRows+2) != panelHeight(panelMaxRows) {
		t.Errorf("height is not capped at %d rows", panelMaxRows)
	}
}

func TestTrayPercent(t *testing.T) {
	tasks := []download.Task{
		{Status: download.StatusDownloading, Size: 100, Downloaded: 25},
		{Status: download.StatusDownloading, Size: 300, Downloaded: 75},
		// A finished download is not part of what is on its way.
		{Status: download.StatusCompleted, Size: 1000, Downloaded: 1000},
	}
	if got, want := trayPercent(tasks), "\u200725%"; got != want {
		t.Errorf("trayPercent = %q, want %q", got, want)
	}
	if got := trayPercent([]download.Task{{Status: download.StatusDownloading, Size: -1}}); got != "" {
		t.Errorf("trayPercent of an unknown size = %q, want empty", got)
	}
	if got := trayPercent(nil); got != "" {
		t.Errorf("trayPercent of no tasks = %q, want empty", got)
	}
}

func TestTrayTipListsRunningDownloads(t *testing.T) {
	tasks := []download.Task{
		{FileName: "idea.dmg", Status: download.StatusDownloading, Size: 1000, Downloaded: 400},
		{FileName: "old.zip", Status: download.StatusCompleted, Size: 10, Downloaded: 10},
	}
	if got, want := trayTip(tasks), "Segra\nidea.dmg 40%"; got != want {
		t.Errorf("trayTip = %q, want %q", got, want)
	}
	if got := trayTip(nil); got != "Segra" {
		t.Errorf("trayTip of no tasks = %q", got)
	}
}

func TestPanelShowsDownload(t *testing.T) {
	a := newTestApp(t)
	a.mgr.Load([]download.Task{{
		ID:          "one",
		URL:         "https://example.com/idea.dmg",
		FileName:    "idea.dmg",
		Dir:         a.settings.DownloadDir,
		Size:        4096,
		Downloaded:  1024,
		Status:      download.StatusPaused,
		Connections: 4,
		Segments:    []download.SegmentProgress{{Start: 0, Done: 1024, Total: 4096}},
	}})

	tt := ui.NewTester(a.panelView, panelWidth, panelHeight(1))
	if !tt.HasText("idea.dmg") {
		t.Fatalf("texts %q do not show the download", tt.Texts())
	}
	if !tt.HasText("25%") {
		t.Fatalf("texts %q do not show its share", tt.Texts())
	}
	if !tt.HasText(locales.PauseAll()) {
		t.Fatalf("texts %q do not show the buttons", tt.Texts())
	}
	assertPanelFits(t, tt, panelHeight(1))
}

func TestPanelRowOpensTheDownload(t *testing.T) {
	a := newTestApp(t)
	a.mgr.Load([]download.Task{
		{ID: "a", FileName: "first.iso", Dir: a.settings.DownloadDir, Status: download.StatusPaused, Size: 4096, Downloaded: 1024},
		{ID: "b", FileName: "second.iso", Dir: a.settings.DownloadDir, Status: download.StatusPaused, Size: 4096, Downloaded: 1024},
	})
	a.refresh()

	tt := ui.NewTester(a.panelView, panelWidth, panelHeight(2))
	if err := tt.Click("first.iso"); err != nil {
		t.Fatal(err)
	}
	// The list shows the newest download first, so the older one is the
	// second row.
	if a.selected != 1 || a.rows[a.selected].ID != "a" {
		t.Fatalf("selected = %d of %d rows", a.selected, len(a.rows))
	}
	if a.view != "all" {
		t.Errorf("view = %q, want all", a.view)
	}
}

func TestPanelIdle(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.panelView, panelWidth, panelHeight(0))
	if !tt.HasText(locales.PanelIdle()) {
		t.Fatalf("texts %q do not show the idle line", tt.Texts())
	}
	assertPanelFits(t, tt, panelHeight(0))
}

// assertPanelFits checks that the last row of the panel is inside the height
// and the width panelHeight and panelWidth give it: native UI cannot measure
// itself, so the arithmetic has to hold, in every language the app speaks.
func assertPanelFits(t *testing.T, tt *ui.Tester, height int) {
	t.Helper()
	rect, ok := tt.Find(locales.TrayOpen())
	if !ok {
		t.Fatalf("texts %q do not show the open button", tt.Texts())
	}
	if bottom := rect.Y + rect.H; bottom > float32(height) {
		t.Errorf("the buttons end at %v, past the %d the panel gives them", bottom, height)
	}
	if right := rect.X + rect.W; right > panelWidth-panelPad {
		t.Errorf("the buttons end at %v, past the %d the panel gives them", right, panelWidth-panelPad)
	}
}

func TestPanelFitsItsSize(t *testing.T) {
	for rows := 0; rows <= panelMaxRows; rows++ {
		var tasks []download.Task
		for i := 0; i < rows; i++ {
			tasks = append(tasks, download.Task{
				ID:          fmt.Sprint(i),
				FileName:    "idea-2026.3-aarch64.dmg",
				Status:      download.StatusDownloading,
				Size:        4096,
				Downloaded:  1024,
				Connections: 32,
				Segments:    []download.SegmentProgress{{Start: 0, Done: 1024, Total: 4096}},
			})
		}
		a := newTestApp(t)
		a.mgr.Load(tasks)
		height := panelHeight(rows)
		tt := ui.NewTester(a.panelView, panelWidth, height)
		assertPanelFits(t, tt, height)
	}
}
