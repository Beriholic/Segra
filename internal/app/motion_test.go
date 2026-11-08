package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

func TestHeatTweenLightsFromWhatItShowed(t *testing.T) {
	h := &heatTween{}
	now := time.Now()
	cells := make([]blockState, chunkRows*chunkCols)
	if k := h.update("a", cells, now); k != 1 {
		t.Fatalf("first frame k = %v, want 1", k)
	}

	next := append([]blockState(nil), cells...)
	next[0], next[1] = blockDone, blockActive
	// The frame that sees the change starts the tween where the grid showed.
	change := now.Add(time.Second)
	if k := h.update("a", next, change); k != 0 {
		t.Fatalf("the frame of the change k = %v, want 0", k)
	}
	if k := h.update("a", next, change.Add(heatFade/2)); k <= 0 || k >= 1 {
		t.Fatalf("mid tween k = %v, want between 0 and 1", k)
	}
	if h.from[0] != blockPending {
		t.Errorf("cell 0 came from %v, want the pending state it showed", h.from[0])
	}
	if h.to[0] != blockDone {
		t.Errorf("cell 0 goes to %v, want done", h.to[0])
	}
	if k := h.update("a", next, change.Add(heatFade)); k != 1 {
		t.Errorf("settled k = %v, want 1", k)
	}
}

func TestHeatTweenStartsOverForAnotherDownload(t *testing.T) {
	h := &heatTween{}
	now := time.Now()
	cells := make([]blockState, chunkRows*chunkCols)
	h.update("a", cells, now)
	other := append([]blockState(nil), cells...)
	other[0] = blockDone
	// Another download's map is not a change of this one's: it shows at
	// once rather than tweening from a grid that belongs to another file.
	if k := h.update("b", other, now.Add(time.Millisecond)); k != 1 {
		t.Errorf("k = %v for another download, want 1", k)
	}
}

func TestSparkFlowMorphsBetweenSamples(t *testing.T) {
	a := &App{}
	now := time.Now()
	if _, moving := a.sparkFlow([]float64{0, 10}, now); moving {
		t.Fatal("the first series moves, want it shown at once")
	}
	// The frame that sees the new samples starts the morph where the curve
	// drew, and the curve flows to them from there.
	change := now.Add(time.Second)
	if vals, moving := a.sparkFlow([]float64{0, 20}, change); !moving || vals[1] != 10 {
		t.Fatalf("the frame of the change = %v, moving %v, want the curve where it drew", vals[1], moving)
	}
	vals, moving := a.sparkFlow([]float64{0, 20}, change.Add(sparkMorph/2))
	if !moving {
		t.Fatal("a changed series is not moving")
	}
	if vals[1] <= 10 || vals[1] >= 20 {
		t.Errorf("value = %v, want it between the sample it showed and the one it has", vals[1])
	}
	done, moving := a.sparkFlow([]float64{0, 20}, change.Add(sparkMorph))
	if moving {
		t.Error("the curve still moves after the morph")
	}
	if done[1] != 20 {
		t.Errorf("settled value = %v, want 20", done[1])
	}
}

func TestNoteTasksKeepsWhenADownloadShowed(t *testing.T) {
	a := &App{}
	now := time.Now()
	a.noteTasks([]download.Task{{ID: "a"}}, now)
	a.noteTasks([]download.Task{{ID: "a"}, {ID: "b"}}, now.Add(time.Second))
	if at := a.seenAt["a"]; !at.Equal(now) {
		t.Errorf("a first showed at %v, want %v", at, now)
	}
	if !a.fresh("b", freshFor) {
		t.Error("b is not fresh though it just showed")
	}
	a.seenAt["c"] = now.Add(-2 * freshFor)
	if a.fresh("c", freshFor) {
		t.Error("c is fresh though it showed long ago")
	}
}

func TestNoteTasksForgetsDownloadsGone(t *testing.T) {
	a := &App{}
	now := time.Now()
	var many []download.Task
	for i := 0; i < 40; i++ {
		many = append(many, download.Task{ID: string(rune('a' + i))})
	}
	a.noteTasks(many, now)
	a.noteTasks([]download.Task{{ID: "a"}}, now)
	if _, ok := a.seenAt["b"]; ok {
		t.Error("a download gone from the list is still remembered")
	}
}

// TestMotionSettles checks that the motion of a list that loses rows and a
// window that changes views comes to rest: a view that never stops asking for
// frames would draw forever.
func TestMotionSettles(t *testing.T) {
	a := newTestApp(t)
	var tasks []download.Task
	for i := 0; i < 6; i++ {
		tasks = append(tasks, download.Task{
			ID: fmt.Sprint("s", i), URL: "https://example.com/f",
			FileName: fmt.Sprintf("f-%02d.iso", i), Dir: a.settings.DownloadDir,
			Size: 100, Downloaded: 50, Status: download.StatusPaused,
		})
	}
	a.mgr.Load(tasks)
	tt := ui.NewTester(a.viewFn, 1180, 760)
	tt.Advance(500 * time.Millisecond)

	for _, id := range []string{"s1", "s3"} {
		for _, task := range a.mgr.List() {
			if task.ID == id {
				a.remove(task, false)
			}
		}
	}
	tt.Advance(400 * time.Millisecond)
	if err := tt.Click(locales.Completed()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(80 * time.Millisecond)
	if err := tt.Click(locales.All()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(1500 * time.Millisecond)
	if tt.FramePending() {
		t.Error("frames are still pending after the motion settles")
	}
	if !tt.HasText("f-00.iso") {
		t.Errorf("texts %q do not show the remaining downloads", tt.Texts())
	}
}
