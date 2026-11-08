package download

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if task, ok := m.Get(id); ok && task.Status == want {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	task, _ := m.Get(id)
	t.Fatalf("task status = %q, want %q (error %q, downloaded %d/%d)", task.Status, want, task.Error, task.Downloaded, task.Size)
	return task
}

func testServer(payload []byte, delay time.Duration, ranges bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"test-v1"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		if ranges {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		rangeHeader := r.Header.Get("Range")
		if !ranges || rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload)
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		end = min(end, len(payload)-1)
		if start > end {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(payload)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		chunk := payload[start : end+1]
		flusher, _ := w.(http.Flusher)
		for len(chunk) > 0 {
			n := min(4096, len(chunk))
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
			chunk = chunk[n:]
			if flusher != nil {
				flusher.Flush()
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}))
}

func testManager(t *testing.T, dir string) (*Manager, chan Event) {
	t.Helper()
	events := make(chan Event, 16)
	m := New(&http.Client{Timeout: 0}, Config{Dir: dir, Connections: 4, MaxConcurrent: 2}, nil)
	m.SetOnEvent(func(e Event) {
		select {
		case events <- e:
		default:
		}
	})
	t.Cleanup(m.Close)
	return m, events
}

func TestWorkerPoolBalancesChunks(t *testing.T) {
	// 16 MB over 4 connections: four ranges of 4 MB, one per worker.
	payload := bytes.Repeat([]byte("0123456789abcdef"), 1<<20)
	var requests, violations int32
	sem := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			atomic.AddInt32(&violations, 1)
		}
		atomic.AddInt32(&requests, 1)
		w.Header().Set("ETag", `"test-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		end = min(end, len(payload)-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		chunk := payload[start : end+1]
		for len(chunk) > 0 {
			n := min(64<<10, len(chunk))
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
			chunk = chunk[n:]
			if len(chunk) > 0 {
				time.Sleep(3 * time.Millisecond)
			}
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/big.bin", Connections: 4})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 60*time.Second)
	if done.Downloaded != int64(len(payload)) {
		t.Fatalf("downloaded %d, want %d", done.Downloaded, len(payload))
	}
	got, err := os.ReadFile(filepath.Join(dir, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded bytes differ from payload")
	}
	if got := atomic.LoadInt32(&violations); got != 0 {
		t.Fatalf("saw %d requests beyond the 4 workers", got)
	}
	if got := atomic.LoadInt32(&requests); got < 4 {
		t.Fatalf("requests = %d, want at least one per connection", got)
	}
}

func TestSegmentedDownload(t *testing.T) {
	payload := bytes.Repeat([]byte("segra-"), 200_000) // 1.2 MB
	srv := testServer(payload, 0, true)
	defer srv.Close()
	dir := t.TempDir()
	m, events := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/file.bin"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 20*time.Second)
	if done.Size != int64(len(payload)) || done.Downloaded != int64(len(payload)) {
		t.Fatalf("size = %d/%d, want %d", done.Downloaded, done.Size, len(payload))
	}
	got, err := os.ReadFile(filepath.Join(dir, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded %d bytes that differ from the payload", len(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "file.bin.part")); !os.IsNotExist(err) {
		t.Fatalf(".part file still exists: %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Kind == EventCompleted {
				return
			}
			if e.Kind == EventFailed {
				t.Fatalf("task failed: %s", e.Task.Error)
			}
		case <-deadline:
			t.Fatal("no completion event")
		}
	}
}

func TestPauseAndResume(t *testing.T) {
	payload := bytes.Repeat([]byte("p"), 512*1024)
	srv := testServer(payload, 5*time.Millisecond, true)
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/big.bin", Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		cur, _ := m.Get(task.ID)
		if cur.Downloaded > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.Pause(task.ID)
	paused := waitStatus(t, m, task.ID, StatusPaused, 5*time.Second)
	if paused.Downloaded <= 0 {
		t.Fatalf("paused task has no progress: %d", paused.Downloaded)
	}
	part := filepath.Join(dir, "big.bin.part")
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("partial file missing: %v", err)
	}
	if _, err := os.Stat(MetadataPath(part)); err != nil {
		t.Fatalf("metadata missing: %v", err)
	}

	m.Resume(task.ID)
	waitStatus(t, m, task.ID, StatusCompleted, 20*time.Second)
	got, err := os.ReadFile(filepath.Join(dir, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("resumed file has %d bytes, want %d", len(got), len(payload))
	}
}

func TestServerWithoutRanges(t *testing.T) {
	payload := []byte(strings.Repeat("x", 200_000))
	srv := testServer(payload, 0, false)
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/plain.bin"})
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, task.ID, StatusCompleted, 10*time.Second)
	got, err := os.ReadFile(filepath.Join(dir, "plain.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("single-stream bytes differ")
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	payload := []byte("hello")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="report 2026.pdf"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/download?id=7"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 10*time.Second)
	if done.FileName != "report 2026.pdf" {
		t.Fatalf("filename = %q", done.FileName)
	}
	if _, err := os.Stat(filepath.Join(dir, "report 2026.pdf")); err != nil {
		t.Fatal(err)
	}
}

// TestFilenameAfterRedirect is the shape a GitHub release download takes:
// the first URL redirects to a signed path, and the name arrives with the
// headers of the redirect. A name guessed from the first URL would call the
// file after the signature.
func TestFilenameAfterRedirect(t *testing.T) {
	payload := []byte("hello")
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/download/v1/app.dmg", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/objects/b674b4e8-15da-4d0e-a795-7256df96b014", http.StatusFound)
	})
	mux.HandleFunc("/objects/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="app-1.0.dmg"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/releases/download/v1/app.dmg"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 10*time.Second)
	if done.FileName != "app-1.0.dmg" {
		t.Fatalf("filename = %q", done.FileName)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-1.0.dmg")); err != nil {
		t.Fatal(err)
	}
}

func TestUniqueNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.zip"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := UniqueName(dir, "a.zip"); got != "a (2).zip" {
		t.Fatalf("UniqueName = %q", got)
	}
	if got := Sanitize("../etc/passwd"); strings.ContainsAny(got, "/\\") {
		t.Fatalf("Sanitize left separators: %q", got)
	}
}

func TestRejectsBadURL(t *testing.T) {
	m, _ := testManager(t, t.TempDir())
	for _, raw := range []string{"", "ftp://example.com/a", "not a url"} {
		if _, err := m.Create(CreateRequest{URL: raw}); err == nil {
			t.Fatalf("Create(%q) succeeded", raw)
		}
	}
}

func TestProgressIsReported(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 1024*1024)
	srv := testServer(payload, 5*time.Millisecond, true)
	defer srv.Close()
	m, _ := testManager(t, t.TempDir())
	task, err := m.Create(CreateRequest{URL: srv.URL + "/p.bin"})
	if err != nil {
		t.Fatal(err)
	}
	sawProgress := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := m.Get(task.ID)
		if cur.Downloaded > 0 && cur.Status == StatusDownloading {
			sawProgress = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawProgress {
		t.Fatal("no progress while downloading")
	}
	waitStatus(t, m, task.ID, StatusCompleted, 10*time.Second)
	if io.EOF == nil {
		t.Fatal("unreachable")
	}
}

func TestExistingFileGetsUniqueName(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 4096)
	srv := testServer(payload, 0, true)
	defer srv.Close()
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.bin")
	if err := os.WriteFile(existing, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, _ := testManager(t, dir)
	task, err := m.Create(CreateRequest{URL: srv.URL + "/report.bin", FileName: "report.bin"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 10*time.Second)
	if done.FileName != "report (2).bin" {
		t.Fatalf("fileName = %q, want report (2).bin", done.FileName)
	}
	if got, _ := os.ReadFile(existing); string(got) != "keep me" {
		t.Fatalf("existing file changed: %q", got)
	}
}

func TestDoesNotResumeAnotherTasksPart(t *testing.T) {
	payload := bytes.Repeat([]byte("y"), 256*1024)
	srv := testServer(payload, 10*time.Millisecond, true)
	defer srv.Close()
	dir := t.TempDir()
	m, _ := testManager(t, dir)

	first, err := m.Create(CreateRequest{URL: srv.URL + "/same.bin", Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, first.ID, StatusDownloading, 5*time.Second)
	// Pause once bytes are on disk: a task paused before it opened its part
	// file leaves nothing behind for the second one to collide with.
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, ok := m.Get(first.ID)
		if ok && task.Downloaded > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first task never wrote a byte")
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.Pause(first.ID)
	waitStatus(t, m, first.ID, StatusPaused, 5*time.Second)

	second, err := m.Create(CreateRequest{URL: srv.URL + "/same.bin", Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, second.ID, StatusCompleted, 10*time.Second)
	if done.FileName == "same.bin" {
		t.Fatalf("second task reused the first task's part: %q", done.FileName)
	}
	if got, err := os.ReadFile(done.Path()); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("second download corrupt: %v", err)
	}
}

// A task the user paused stays paused across launches; only the one that was
// on its way when the app quit continues.
func TestPausedTaskStaysPausedOnLaunch(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 64*1024)
	srv := testServer(payload, 0, true)
	defer srv.Close()
	dir := t.TempDir()

	// What tasks.json holds after a quit: the task that was running still
	// says so, the one the user paused says paused.
	m, _ := testManager(t, dir)
	m.Load([]Task{
		{ID: "running", URL: srv.URL + "/running.bin", FileName: "running.bin", Dir: dir, Status: StatusDownloading, Connections: 2},
		{ID: "paused", URL: srv.URL + "/paused.bin", FileName: "paused.bin", Dir: dir, Status: StatusPaused, Connections: 2},
	})

	running, _ := m.Get("running")
	if running.Status != StatusPaused || !running.Interrupted {
		t.Fatalf("the task that was running loaded as %q, interrupted %v", running.Status, running.Interrupted)
	}
	held, _ := m.Get("paused")
	if held.Status != StatusPaused || held.Interrupted {
		t.Fatalf("the task the user paused loaded as %q, interrupted %v", held.Status, held.Interrupted)
	}

	m.ResumeInterrupted()

	done := waitStatus(t, m, "running", StatusCompleted, 10*time.Second)
	if got, err := os.ReadFile(done.Path()); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("the resumed download is corrupt: %v", err)
	}
	if got, _ := m.Get("paused"); got.Status != StatusPaused {
		t.Fatalf("the task the user paused is %q, want paused", got.Status)
	}
}

// A connection that stops delivering bytes must be dropped and reopened by
// the health monitor instead of hanging until the much longer stall timeout.
func TestStalledConnectionIsReplaced(t *testing.T) {
	payload := make([]byte, 2<<20)
	for i := range payload {
		payload[i] = byte(i * 7)
	}
	var largeRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stall-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}
		end = min(end, int64(len(payload)-1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		flusher, _ := w.(http.Flusher)

		// Give the probe its single byte without stalling.
		if start == 0 && end == 0 {
			_, _ = w.Write(payload[:1])
			return
		}
		if largeRequests.Add(1) == 1 {
			_, _ = w.Write(payload[start : start+(512<<10)])
			if flusher != nil {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := New(&http.Client{}, Config{Dir: dir, Connections: 1, MaxConcurrent: 1}, nil)
	t.Cleanup(m.Close)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/file.bin", Dir: dir, Connections: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 30*time.Second)
	if got := largeRequests.Load(); got < 2 {
		t.Fatalf("large requests = %d, want at least 2 (one stalled + one retry)", got)
	}
	data, err := os.ReadFile(done.Path())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("downloaded %d bytes, content mismatch", len(data))
	}
}

// Pausing a download must hand its scheduler slot to the next queued task
// right away instead of waiting for the cancelled worker to unwind.
func TestPauseFreesQueueSlot(t *testing.T) {
	slow := testServer(bytes.Repeat([]byte("a"), 8<<20), 30*time.Millisecond, true)
	defer slow.Close()
	fast := testServer(bytes.Repeat([]byte("b"), 1<<20), 0, true)
	defer fast.Close()

	dir := t.TempDir()
	m := New(&http.Client{}, Config{Dir: dir, Connections: 2, MaxConcurrent: 1}, nil)
	t.Cleanup(m.Close)

	first, err := m.Create(CreateRequest{URL: slow.URL + "/a.bin", Dir: dir, Connections: 1})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	waitStatus(t, m, first.ID, StatusDownloading, 5*time.Second)
	m.Pause(first.ID)

	second, err := m.Create(CreateRequest{URL: fast.URL + "/b.bin", Dir: dir, Connections: 1})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, ok := m.Get(second.ID); ok && got.Status != StatusQueued {
			waitStatus(t, m, second.ID, StatusCompleted, 10*time.Second)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := m.Get(second.ID)
	t.Fatalf("second task stayed %q after pausing the first", got.Status)
}

func TestConnectionsAreClampedTo64(t *testing.T) {
	dir := t.TempDir()
	m := New(&http.Client{}, Config{Dir: dir, Connections: 32, MaxConcurrent: 1}, nil)
	t.Cleanup(m.Close)
	for _, want := range []int{64, 64} {
		task, err := m.Create(CreateRequest{URL: "http://127.0.0.1:1/file", Connections: 100})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if task.Connections != want {
			t.Fatalf("connections = %d, want %d", task.Connections, want)
		}
	}
}

// The downloader must own its connections: one TCP connection per worker,
// and never the caller's transport. Sharing a single HTTP/2 connection
// starves every worker at once when the CDN stops returning window updates,
// which is a download stuck at zero bytes, not a slow one.
func TestDownloaderOwnsItsConnections(t *testing.T) {
	caller := &http.Transport{ForceAttemptHTTP2: true, MaxIdleConnsPerHost: 7}
	m := New(&http.Client{Transport: caller}, Config{Dir: t.TempDir(), Connections: 8}, nil)
	t.Cleanup(m.Close)

	tr, ok := m.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("manager transport = %T, want *http.Transport", m.client.Transport)
	}
	if tr == caller {
		t.Fatal("manager reconfigured the caller's transport")
	}
	if tr.ForceAttemptHTTP2 || len(tr.TLSNextProto) != 0 {
		t.Fatal("manager allows HTTP/2: the workers would share one connection's flow-control window")
	}
	if tr.MaxIdleConnsPerHost != 7 {
		t.Fatalf("idle connections per host = %d, want the caller's 7", tr.MaxIdleConnsPerHost)
	}
	// HTTP/1.1 must still be offered over ALPN. Turning h2 off makes Go
	// send no ALPN at all, and a CDN that wants one resets the handshake:
	// github.com answered a download with EOF until the downloader named
	// http/1.1 itself.
	if tr.TLSClientConfig == nil || len(tr.TLSClientConfig.NextProtos) == 0 {
		t.Fatal("manager sends no ALPN: some hosts close the connection instead of answering")
	}
	if got := tr.TLSClientConfig.NextProtos[0]; got != "http/1.1" {
		t.Fatalf("ALPN = %q, want http/1.1", got)
	}
	// The caller's transport is shared with the rest of the app, which may
	// still want h2. (Clone registers h2 on it lazily, the way any first
	// use does; that is Go's initialisation, not our change.)
	if !caller.ForceAttemptHTTP2 {
		t.Fatal("manager disabled HTTP/2 on the caller's transport")
	}
}

// A server that takes the connection and then says nothing must end the
// download with an error. It used to sit in "downloading" at zero bytes
// forever, because the worker's own cancellation was read as a pause.
func TestSilentServerFailsInsteadOfHanging(t *testing.T) {
	shrink := func(dst *time.Duration, v time.Duration) {
		old := *dst
		*dst = v
		t.Cleanup(func() { *dst = old })
	}
	oldBudget := stallBudget
	stallBudget = 300 * time.Millisecond
	t.Cleanup(func() { stallBudget = oldBudget })
	shrink(&healthInterval, 20*time.Millisecond)
	shrink(&healthGrace, 40*time.Millisecond)
	shrink(&stallAfter, 20*time.Millisecond)
	shrink(&stallTimeout, 100*time.Millisecond)
	shrink(&headerTimeout, 100*time.Millisecond)

	payload := make([]byte, 2<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"silent-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		end = min(end, int64(len(payload)-1))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		if start == 0 && end == 0 {
			_, _ = w.Write(payload[:1])
			return
		}
		// Accept the range, send nothing, keep the connection open.
		<-r.Context().Done()
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := New(&http.Client{}, Config{Dir: dir, Connections: 4, MaxConcurrent: 1}, nil)
	t.Cleanup(m.Close)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/file.bin", Dir: dir, Connections: 4})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	failed := waitStatus(t, m, task.ID, StatusFailed, 20*time.Second)
	if !strings.Contains(failed.Error, "stalled") {
		t.Fatalf("error = %q, want it to name the stalled connection", failed.Error)
	}
}

// A burst of connection failures -- the TLS handshake timing out, the
// socket reset before the headers -- must not kill the download. A hotspot
// produces exactly that, and the task used to fail at the first quiet
// second: four reconnects in a row was all it took.
func TestConnectionDropoutsDoNotFailDownload(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 64<<10) // 1 MiB
	var dropped atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-0" && dropped.Add(1) <= 16 {
			// Gone before a byte of response, the way a timed-out
			// handshake or a reset connection looks to the client.
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
			return
		}
		w.Header().Set("ETag", `"test-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Header.Get("Range") == "bytes=0-0" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(payload)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		end = min(end, len(payload)-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := New(&http.Client{}, Config{Dir: dir, Connections: 4, MaxConcurrent: 1}, nil)
	t.Cleanup(m.Close)

	task, err := m.Create(CreateRequest{URL: srv.URL + "/file.bin", Dir: dir, Connections: 4})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	done := waitStatus(t, m, task.ID, StatusCompleted, 60*time.Second)
	if done.Downloaded != int64(len(payload)) {
		t.Fatalf("downloaded %d, want %d", done.Downloaded, len(payload))
	}
	got, err := os.ReadFile(filepath.Join(dir, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("downloaded bytes differ from payload")
	}
	if n := dropped.Load(); n < 16 {
		t.Fatalf("server dropped %d connections, want the full 16", n)
	}
}

// Resuming must use the redirect the probe just walked, not the URL saved
// in the .part file: CDN links are signed and expire, and a task paused
// overnight came back to a 403.
func TestResumeRefreshesSignedURL(t *testing.T) {
	payload := bytes.Repeat([]byte("s"), 512*1024)
	var token atomic.Int32
	token.Store(1)
	// The first chunk of every range goes out, then the connection waits for
	// the test: a download that can finish before the pause proves nothing
	// about resuming one.
	var hold atomic.Bool
	hold.Store(true)
	gate := make(chan struct{})
	var gateOnce sync.Once
	openGate := func() { gateOnce.Do(func() { close(gate) }) }
	defer openGate()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file.bin" {
			http.Redirect(w, r, fmt.Sprintf("/cdn/file.bin?token=%d", token.Load()), http.StatusFound)
			return
		}
		if r.URL.Query().Get("token") != strconv.Itoa(int(token.Load())) {
			http.Error(w, "link expired", http.StatusForbidden)
			return
		}
		w.Header().Set("ETag", `"signed-v1"`)
		w.Header().Set("Accept-Ranges", "bytes")
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		end = min(end, len(payload)-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		chunk := payload[start : end+1]
		flusher, _ := w.(http.Flusher)
		if n := min(4096, len(chunk)); n > 0 {
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
			chunk = chunk[n:]
			if flusher != nil {
				flusher.Flush()
			}
		}
		if hold.Load() {
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		for len(chunk) > 0 {
			n := min(4096, len(chunk))
			if _, err := w.Write(chunk[:n]); err != nil {
				return
			}
			chunk = chunk[n:]
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(3 * time.Millisecond)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	m, _ := testManager(t, dir)
	task, err := m.Create(CreateRequest{URL: srv.URL + "/file.bin", Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		cur, _ := m.Get(task.ID)
		if cur.Downloaded > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.Pause(task.ID)
	waitStatus(t, m, task.ID, StatusPaused, 5*time.Second)

	// The link in the .part file is now dead; only a fresh probe helps.
	hold.Store(false)
	openGate()
	token.Store(2)
	m.Resume(task.ID)
	waitStatus(t, m, task.ID, StatusCompleted, 20*time.Second)
	got, err := os.ReadFile(filepath.Join(dir, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("resumed file has %d bytes, want %d", len(got), len(payload))
	}
}
