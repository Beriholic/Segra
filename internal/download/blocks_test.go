package download

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"
)

func TestProgressReportsBlockShares(t *testing.T) {
	dir := t.TempDir()
	payload := bytes.Repeat([]byte("x"), 1<<20)
	srv := testServer(payload, 2*time.Millisecond, true)
	defer srv.Close()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/block.bin", Connections: 4})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := m.Get(task.ID)
		if len(got.Segments) > 0 && got.Downloaded > 0 {
			var done int64
			for _, s := range got.Segments {
				done += s.Done
				if s.Total != int64(len(payload))/int64(len(got.Segments)) {
					t.Fatalf("block total = %d, want an even share of %d", s.Total, len(payload))
				}
				if s.Start+s.Total > int64(len(payload)) {
					t.Fatalf("block %d starts at %d with %d bytes, past the end", s.Start, s.Total, len(payload))
				}
			}
			if done != got.Downloaded {
				t.Fatalf("blocks hold %d bytes, task says %d", done, got.Downloaded)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no block progress reached the task")
}

func TestLoadRestoresBlockProgress(t *testing.T) {
	dir := t.TempDir()
	task := Task{
		ID: "abc", URL: "https://example.com/f.bin", FileName: "f.bin",
		Dir: dir, Size: 1000, Status: StatusPaused, Connections: 2,
	}
	meta := &taskMeta{
		TaskID: "abc", Size: 1000,
		Segments: []segmentState{
			{Start: 0, End: 499, Done: 500},
			{Start: 500, End: 999, Done: 200},
		},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(MetadataPath(task.PartPath()), meta); err != nil {
		t.Fatal(err)
	}

	m, _ := testManager(t, dir)
	m.Load([]Task{task})

	got, ok := m.Get("abc")
	if !ok {
		t.Fatal("task was not loaded")
	}
	if len(got.Segments) != 2 {
		t.Fatalf("segments = %v, want two", got.Segments)
	}
	if got.Segments[0].Start != 0 || got.Segments[0].Done != 500 || got.Segments[0].Total != 500 {
		t.Fatalf("first block = %+v, want 500/500", got.Segments[0])
	}
	if got.Segments[1].Done != 200 || got.Segments[1].Total != 500 {
		t.Fatalf("second block = %+v, want 200/500", got.Segments[1])
	}
}

func TestPlanSegmentsCutsOneRangePerConnection(t *testing.T) {
	segs := planSegments(4<<20, 4)
	if len(segs) != 4 {
		t.Fatalf("ranges = %d, want one per connection", len(segs))
	}
	for i, s := range segs {
		if s.Start != int64(i)<<20 || s.End != int64(i+1)<<20-1 {
			t.Fatalf("range %d = %d-%d, want its own quarter", i, s.Start, s.End)
		}
	}
	if got := planSegments(3, 8); len(got) != 3 {
		t.Fatalf("ranges = %d, want one per byte of a 3 byte file", len(got))
	}
}

func TestStealWorkSplitsTheRangeWithTheMostLeft(t *testing.T) {
	meta := &taskMeta{Size: 8 << 20, Segments: []segmentState{
		{Start: 0, End: 4<<20 - 1},
		{Start: 4 << 20, End: 8<<20 - 1, Done: 3 << 20},
	}}
	tracker := newHealthTracker(1)
	tracker.begin(0, 0, time.Now(), func() {})
	defer tracker.end(0)

	var mu sync.Mutex
	index, ok := stealWork(meta, &mu, tracker)
	if !ok {
		t.Fatal("no range was stolen")
	}
	if index != 2 {
		t.Fatalf("stolen range = %d, want 2", index)
	}
	if got := meta.Segments[0].End; got != 2<<20-1 {
		t.Fatalf("owner keeps up to %d, want the first half", got)
	}
	tail := meta.Segments[2]
	if tail.Start != 2<<20 || tail.End != 4<<20-1 {
		t.Fatalf("tail = %d-%d, want the back half of the busy range", tail.Start, tail.End)
	}
}

func TestStealWorkLeavesSmallTailsAlone(t *testing.T) {
	meta := &taskMeta{Size: 1 << 20, Segments: []segmentState{
		{Start: 0, End: 1<<20 - 1},
	}}
	tracker := newHealthTracker(1)
	tracker.begin(0, 0, time.Now(), func() {})
	defer tracker.end(0)

	var mu sync.Mutex
	if _, ok := stealWork(meta, &mu, tracker); ok {
		t.Fatal("stole a range with too little left")
	}
}
