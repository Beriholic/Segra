package download

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func (m *Manager) prepare(ctx context.Context, t *Task) (*taskMeta, string, string, error) {
	in := CreateRequest{
		URL: t.URL, Dir: t.Dir, Referer: t.Referer, UserAgent: t.UserAgent,
		Cookie: t.Cookie, Connections: t.Connections,
	}
	probe, err := probeWithRetry(ctx, m.client, in)
	if err != nil {
		return nil, "", "", err
	}

	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return nil, "", "", err
	}

	m.mu.Lock()
	name := strings.TrimSpace(t.FileName)
	if name == "" {
		name = Filename(probe.FinalURL, probe.Disposition)
	} else {
		name = Sanitize(name)
	}
	t.FileName = name
	m.mu.Unlock()

	part := t.PartPath()
	metaPath := MetadataPath(part)

	if old, err := readMeta(metaPath); err == nil && old.TaskID == t.ID {
		if _, statErr := os.Stat(part); statErr == nil && metaMatches(old, probe) {
			if t.Cookie == "" {
				t.Cookie = old.Cookie
			}
			m.mu.Lock()
			t.Size = old.Size
			t.Downloaded = old.downloaded()
			t.Resumable = true
			t.FinalURL = probe.FinalURL
			t.ETag = probe.ETag
			t.LastModified = probe.LastMod
			t.ContentType = probe.ContentType
			m.mu.Unlock()
			return old, part, metaPath, nil
		}
		_ = os.Remove(part)
		removeMeta(metaPath)
	} else if nameTaken(t.Dir, name) {
		m.mu.Lock()
		t.FileName = UniqueName(t.Dir, name)
		m.mu.Unlock()
		part = t.PartPath()
		metaPath = MetadataPath(part)
	}

	segs := planSegments(probe.Size, t.Connections)
	single := !probe.Ranges
	if single {
		segs = []segmentState{{Start: 0, End: probe.Size - 1}}
	}
	meta := &taskMeta{
		Version:     metaVersion,
		TaskID:      t.ID,
		URL:         probe.FinalURL,
		FinalURL:    probe.FinalURL,
		ETag:        probe.ETag,
		LastMod:     probe.LastMod,
		Size:        probe.Size,
		Disposition: probe.Disposition,
		ContentType: probe.ContentType,
		Referer:     t.Referer,
		UserAgent:   t.UserAgent,
		Cookie:      t.Cookie,
		Single:      single,
		Segments:    segs,
	}
	if err := writeMeta(metaPath, meta); err != nil {
		return nil, "", "", err
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, "", "", err
	}
	_ = f.Close()

	m.mu.Lock()
	t.Size = probe.Size
	t.Downloaded = 0
	t.Resumable = probe.Ranges
	t.FinalURL = probe.FinalURL
	t.ETag = probe.ETag
	t.LastModified = probe.LastMod
	t.ContentType = probe.ContentType
	m.mu.Unlock()
	return meta, part, metaPath, nil
}

func metaMatches(m *taskMeta, p *probeResult) bool {
	if m.Size >= 0 && p.Size >= 0 && m.Size != p.Size {
		return false
	}
	if m.ETag != "" && p.ETag != "" && m.ETag != p.ETag {
		return false
	}
	if m.LastMod != "" && p.LastMod != "" && m.LastMod != p.LastMod {
		return false
	}
	if len(m.Segments) == 0 {
		return false
	}
	for _, s := range m.Segments {
		if s.Done < 0 {
			return false
		}
		if l := s.length(); l >= 0 && s.Done > l {
			return false
		}
	}
	if m.Size >= 0 && m.downloaded() > m.Size {
		return false
	}
	return true
}

func (m *Manager) transfer(ctx context.Context, t *Task, meta *taskMeta, f *os.File, metaPath string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	snapshot := func() *taskMeta {
		cp := *meta
		cp.Segments = append([]segmentState(nil), meta.Segments...)
		return &cp
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		meter := &speedMeter{}
		ticks := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				total := meta.downloaded()
				snap := snapshot()
				mu.Unlock()
				rate := meter.update(total, time.Now())
				m.progress(t.ID, total, int64(rate))
				ticks++
				if ticks%10 == 0 {
					_ = writeMeta(metaPath, snap)
				}
			}
		}
	}()

	var err error
	if meta.Single {
		add := func(n int64) {
			mu.Lock()
			meta.Segments[0].Done += n
			mu.Unlock()
		}
		local := meta.Segments[0]
		err = fetchSingle(ctx, m.client, CreateRequest{
			URL: meta.URL, Referer: meta.Referer, UserAgent: meta.UserAgent, Cookie: meta.Cookie,
		}, meta, &local, f, func(n int64) { add(n) })
		cancel()
	} else {
		in := CreateRequest{URL: meta.URL, Referer: meta.Referer, UserAgent: meta.UserAgent, Cookie: meta.Cookie}
		workers := t.Connections
		if workers < 1 {
			workers = 1
		}
		if workers > len(meta.Segments) {
			workers = len(meta.Segments)
		}
		jobs := make(chan int)
		tracker := newHealthTracker(workers)
		healthCtx, stopHealth := context.WithCancel(ctx)
		go tracker.watch(healthCtx)

		var wg sync.WaitGroup
		var errMu sync.Mutex
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := range jobs {
					if e := m.fetchSegment(ctx, worker, i, tracker, in, meta, f, &mu); e != nil {
						errMu.Lock()
						if err == nil {
							err = e
						}
						errMu.Unlock()
						cancel()
						return
					}
				}
			}(w)
		}
		go func() {
			defer close(jobs)
			for i := range meta.Segments {
				select {
				case <-ctx.Done():
					return
				case jobs <- i:
				}
			}
		}()
		wg.Wait()
		stopHealth()
		cancel()
	}
	<-done

	mu.Lock()
	snap := snapshot()
	mu.Unlock()
	_ = writeMeta(metaPath, snap)

	if err != nil && errors.Is(err, context.Canceled) {
		return err
	}
	return err
}

func probeWithRetry(ctx context.Context, client *http.Client, in CreateRequest) (*probeResult, error) {
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		p, err := probe(ctx, client, in)
		if err == nil {
			return p, nil
		}
		last = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var he *httpError
		if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 && he.Status != http.StatusTooManyRequests {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff(attempt + 1)):
		}
	}
	return nil, last
}

// fetchSegment streams one segment. When the health monitor drops the
// connection (stall or a speed far below its peers) the segment is retried
// on a fresh connection, keeping the bytes already written.
func (m *Manager) fetchSegment(ctx context.Context, w int, i int, tracker *healthTracker, in CreateRequest, meta *taskMeta, f *os.File, mu *sync.Mutex) error {
	mu.Lock()
	lastDone := meta.Segments[i].Done
	mu.Unlock()
	reconnects := 0

	for {
		attemptCtx, cancel := context.WithCancel(ctx)
		tracker.begin(w, i, time.Now(), cancel)

		mu.Lock()
		local := meta.Segments[i]
		mu.Unlock()
		add := func(n int64) {
			local.Done += n
			tracker.add(w, n)
			mu.Lock()
			meta.Segments[i].Done = local.Done
			mu.Unlock()
		}
		err := fetchRange(attemptCtx, m.client, in, meta, &local, f, add)
		tracker.end(w)
		cancel()

		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Both a dropped connection and our own watchdog read as cancelled.
		// Neither is a pause: reopen the connection, keeping the bytes
		// already written, and give up with a message once that stops
		// helping. Returning the bare context error here ended in a task
		// that stayed "downloading" forever, since fail() reads it as a
		// pause and stays quiet.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, errStalled) {
			return err
		}
		mu.Lock()
		done := meta.Segments[i].Done
		mu.Unlock()
		if done > lastDone {
			reconnects = 0
		} else {
			reconnects++
		}
		lastDone = done
		if reconnects > maxReconnects {
			return errStalled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
