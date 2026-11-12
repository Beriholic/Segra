package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// addTask puts a task in the manager the way Load does, without the disk.
func addTask(m *Manager, t Task) {
	m.mu.Lock()
	cp := t
	m.tasks[t.ID] = &cp
	m.order = append(m.order, t.ID)
	m.mu.Unlock()
}

func ids(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.ID)
	}
	return out
}

// The history keeps what is recent and what is still on its way, whatever
// the settings say: a paused download is work not yet done, not a record.
func TestPruneHistoryDropsOnlyFinishedRecordsPastTheirDays(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	now := time.Now()
	old := 40 * 24 * time.Hour

	addTask(m, Task{ID: "old-done", Status: StatusCompleted, CompletedAt: now.Add(-old)})
	addTask(m, Task{ID: "new-done", Status: StatusCompleted, CompletedAt: now.Add(-time.Hour)})
	addTask(m, Task{ID: "old-failed", Status: StatusFailed, StartedAt: now.Add(-old)})
	addTask(m, Task{ID: "old-paused", Status: StatusPaused, CreatedAt: now.Add(-old)})
	addTask(m, Task{ID: "old-running", Status: StatusQueued, CreatedAt: now.Add(-old)})

	if n := m.PruneHistory(30*24*time.Hour, 0); n != 2 {
		t.Fatalf("pruned %d records, want the two old finished ones", n)
	}

	for _, id := range ids(m.List()) {
		switch id {
		case "old-done", "old-failed":
			t.Errorf("%s is still in the list", id)
		}
	}
	for _, id := range []string{"new-done", "old-paused", "old-running"} {
		if _, ok := m.Get(id); !ok {
			t.Errorf("%s went with the history, want it kept", id)
		}
	}
}

// The limit keeps the newest records, the way the list shows them, however
// they were started.
func TestPruneHistoryKeepsTheNewestRecords(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	now := time.Now()

	for i, id := range []string{"a", "b", "c", "d", "e"} {
		addTask(m, Task{ID: id, Status: StatusCompleted, CompletedAt: now.Add(-time.Duration(5-i) * time.Hour)})
	}

	if n := m.PruneHistory(0, 2); n != 3 {
		t.Fatalf("pruned %d records, want all but the two newest", n)
	}
	kept := ids(m.List())
	if len(kept) != 2 || kept[0] != "e" || kept[1] != "d" {
		t.Fatalf("kept %v, want the newest first: [e d]", kept)
	}
}

// Tidying the history is not deleting the downloads: a record the list no
// longer shows is still a file in the user's folder.
func TestPruneHistoryKeepsTheFilesTheRecordsNamed(t *testing.T) {
	dir := t.TempDir()
	m, _ := testManager(t, dir)
	file := filepath.Join(dir, "finished.iso")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	addTask(m, Task{
		ID: "finished", FileName: "finished.iso", Dir: dir,
		Status: StatusCompleted, CompletedAt: time.Now().Add(-90 * 24 * time.Hour),
	})

	if n := m.PruneHistory(30*24*time.Hour, 0); n != 1 {
		t.Fatalf("pruned %d records, want 1", n)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the download went with its record: %v", err)
	}
}

// A record from before the app wrote completion times still ages by when it
// started, or when it was created if it never did. One that says nothing at
// all is not old age: only the count can take it.
func TestPruneHistoryAgesByTheBestTimeTheRecordHas(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	now := time.Now()

	addTask(m, Task{ID: "started", Status: StatusFailed, StartedAt: now.Add(-90 * 24 * time.Hour)})
	addTask(m, Task{ID: "created", Status: StatusFailed, CreatedAt: now.Add(-90 * 24 * time.Hour)})

	if n := m.PruneHistory(30*24*time.Hour, 0); n != 2 {
		t.Fatalf("pruned %d records, want both old ones", n)
	}
}

func TestPruneHistoryKeepsARecordThatDoesNotSayWhenItEnded(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	addTask(m, Task{ID: "undated", Status: StatusCompleted})

	if n := m.PruneHistory(time.Hour, 10); n != 0 {
		t.Fatalf("pruned %d records, want the undated one kept", n)
	}
	if _, ok := m.Get("undated"); !ok {
		t.Fatal("a record without a time went as if it were old")
	}
}

// No limit is no limit: the history keeps every record until the user asks
// for it to go.
func TestPruneHistoryWithoutLimitsKeepsEverything(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	addTask(m, Task{ID: "old", Status: StatusCompleted, CreatedAt: time.Now().Add(-1000 * 24 * time.Hour)})

	if n := m.PruneHistory(0, 0); n != 0 {
		t.Fatalf("pruned %d records without a limit, want none", n)
	}
	if _, ok := m.Get("old"); !ok {
		t.Fatal("the record went with no limit set")
	}
}
