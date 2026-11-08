package download

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Pausing frees the concurrency slot at once, so that a queued download goes
// ahead, but the paused task itself has to wait for the worker the pause
// cancelled: two workers on one task both write its .part file and both
// rename it into place, and the second one then fails on a file the first has
// already moved, leaving a finished download marked failed.
func TestPumpLeavesADrainingTaskInTheQueue(t *testing.T) {
	m, _ := testManager(t, t.TempDir())

	m.mu.Lock()
	m.tasks["a"] = &Task{ID: "a", Status: StatusQueued, Dir: t.TempDir()}
	m.queue = []string{"a"}
	m.draining["a"] = true
	m.mu.Unlock()

	m.pump()

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.running["a"]; ok {
		t.Fatal("the task started again under the worker its pause cancelled")
	}
	if len(m.queue) != 1 || m.queue[0] != "a" {
		t.Fatalf("queue = %v, want the task kept until its worker has returned", m.queue)
	}
}

// A task at the head of the queue whose worker is still unwinding must not
// hold the downloads behind it back.
func TestPumpStartsTheTaskBehindADrainingOne(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	dir := t.TempDir()
	m, _ := testManager(t, dir)
	m.mu.Lock()
	m.tasks["a"] = &Task{ID: "a", Status: StatusQueued, Dir: dir, URL: srv.URL + "/a.bin"}
	m.tasks["b"] = &Task{ID: "b", Status: StatusQueued, Dir: dir, URL: srv.URL + "/b.bin"}
	m.queue = []string{"a", "b"}
	m.draining["a"] = true
	m.mu.Unlock()

	m.pump()

	m.mu.Lock()
	_, aStarted := m.running["a"]
	_, bStarted := m.running["b"]
	queued := append([]string(nil), m.queue...)
	m.mu.Unlock()

	if aStarted {
		t.Error("the draining task started again")
	}
	if !bStarted {
		t.Error("the download behind the draining task did not start")
	}
	if len(queued) != 1 || queued[0] != "a" {
		t.Errorf("queue = %v, want only the draining task left", queued)
	}
}
