package download

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// A server that is slow to send its first byte must not be dropped by the
// health monitor: nothing arrived yet, so the header timeout owns the
// attempt. The monitor used to cancel such a worker after its grace period,
// over and over, and the download failed without a byte ever landing. A
// crowded hotspot makes that first byte slow all the time.
func TestSlowFirstByteIsNotDroppedByHealth(t *testing.T) {
	shrink := func(dst *time.Duration, v time.Duration) {
		old := *dst
		*dst = v
		t.Cleanup(func() { *dst = old })
	}
	shrink(&healthInterval, 20*time.Millisecond)
	shrink(&healthGrace, 30*time.Millisecond)
	shrink(&stallAfter, 30*time.Millisecond)
	oldBudget := stallBudget
	stallBudget = time.Second
	t.Cleanup(func() { stallBudget = oldBudget })

	payload := bytes.Repeat([]byte("s"), 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"slow-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		end = min(end, len(payload)-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		if start == 0 && end == 0 {
			_, _ = w.Write(payload[:1])
			return
		}
		// Well past the grace period, well inside the header timeout.
		time.Sleep(150 * time.Millisecond)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dir := t.TempDir()
	m, _ := testManager(t, dir)
	task, err := m.Create(CreateRequest{URL: srv.URL + "/slow.bin", Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 20*time.Second)
	got, err := os.ReadFile(done.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded %d bytes that differ from the payload", len(got))
	}
}
