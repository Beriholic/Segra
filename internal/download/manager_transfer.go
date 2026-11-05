package download

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
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
			// The probe just walked the redirect chain, so it holds a
			// signed URL that is newer than the one in the file: CDNs hand
			// out links that expire, and a download paused overnight must
			// not come back to a 403.
			old.URL = probe.FinalURL
			old.FinalURL = probe.FinalURL
			old.ETag = probe.ETag
			old.LastMod = probe.LastMod
			m.mu.Lock()
			t.Size = old.Size
			t.Downloaded = old.downloaded()
			t.Segments = progressOf(old.Segments)
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
	t.Segments = progressOf(segs)
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
				m.progress(t.ID, total, int64(rate), progressOf(snap.Segments))
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
		// The count is read once, before any worker can append a stolen
		// range to the list.
		ranges := len(meta.Segments)
		jobs := make(chan int, ranges)
		tracker := newHealthTracker(workers)
		healthCtx, stopHealth := context.WithCancel(ctx)
		healthDone := make(chan struct{})
		go func() {
			defer close(healthDone)
			tracker.watch(healthCtx)
		}()

		// closeQueue ends the worker loop once no range has a byte left.
		// Every worker may notice it; only the first close counts.
		var closeOnce sync.Once
		closeQueue := func() { closeOnce.Do(func() { close(jobs) }) }
		allWritten := func() bool {
			mu.Lock()
			defer mu.Unlock()
			for _, s := range meta.Segments {
				if l := s.length(); l < 0 || l > s.Done {
					return false
				}
			}
			return true
		}

		var wg sync.WaitGroup
		var errMu sync.Mutex
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				retry := time.NewTicker(stealRetry)
				defer retry.Stop()

				stop := func(e error) {
					errMu.Lock()
					if err == nil {
						err = e
					}
					errMu.Unlock()
					cancel()
				}
				run := func(i int) bool {
					if e := m.fetchSegment(ctx, worker, i, tracker, in, meta, f, &mu); e != nil {
						stop(e)
						return false
					}
					if allWritten() {
						closeQueue()
					}
					return true
				}

				for {
					select {
					case i, ok := <-jobs:
						if !ok {
							return
						}
						if !run(i) {
							return
						}
						continue
					default:
					}

					// Nothing queued: take the back half of the range with
					// the most left, as Surge's balancer does.
					if i, ok := stealWork(meta, &mu, tracker); ok {
						if !run(i) {
							return
						}
						continue
					}
					select {
					case <-ctx.Done():
						return
					case i, ok := <-jobs:
						if !ok {
							return
						}
						if !run(i) {
							return
						}
					case <-retry.C:
					}
				}
			}(w)
		}
		// Every range goes out at once, one per worker: with the file cut
		// once per connection there is nothing left to queue after that.
		for i := 0; i < ranges; i++ {
			jobs <- i
		}
		wg.Wait()
		stopHealth()
		// Do not leave the loop reading worker state after the transfer.
		<-healthDone
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

// stealRetry is how often a worker with nothing queued looks again for a
// range worth splitting.
const stealRetry = 25 * time.Millisecond

// stealAlign keeps a stolen range on a 4 KiB boundary, as the servers and
// Surge's own splitter do.
const stealAlign = 4 << 10

// minSteal is the least a range must have left on both sides of the split
// before it is worth taking. A variable so a test can shrink it.
var minSteal = int64(1 << 20)

// stealWork takes the back half of the range with the most bytes left and
// returns it for the worker that has nothing queued. The owner's request is
// cancelled and reopens on the shorter half, keeping the bytes it already
// wrote. This is Surge's balancer in miniature: the work goes to the idle
// connection instead of that connection waiting out a slow one.
func stealWork(meta *taskMeta, mu *sync.Mutex, tracker *healthTracker) (int, bool) {
	mu.Lock()
	best, bestLeft := -1, int64(0)
	for i, s := range meta.Segments {
		if left := s.length() - s.Done; left > bestLeft {
			best, bestLeft = i, left
		}
	}
	if best < 0 || bestLeft < 2*minSteal {
		mu.Unlock()
		return 0, false
	}
	seg := meta.Segments[best]
	split := (seg.Done + bestLeft/2) / stealAlign * stealAlign
	if split <= seg.Done || split >= seg.length() {
		mu.Unlock()
		return 0, false
	}
	tail := segmentState{Start: seg.Start + split, End: seg.End}
	meta.Segments[best].End = tail.Start - 1
	if meta.Segments[best].Done > split {
		meta.Segments[best].Done = split
	}
	meta.Segments = append(meta.Segments, tail)
	index := len(meta.Segments) - 1
	mu.Unlock()

	// Interrupt the owner so it reopens on the half it keeps; if it is
	// between attempts it will read the shorter range next time anyway.
	for w, wh := range tracker.workers {
		if wh.active.Load() && int(wh.seg.Load()) == best {
			tracker.cancelAttempt(w)
			break
		}
	}
	return index, true
}

// stallFailure builds what the task shows once a segment has spent
// stallBudget without a byte. A watchdog cut is a stall; a network error
// that never let a byte through is worth naming, minus the URL the task
// already carries and the UI has no room for.
func stallFailure(last error) error {
	var ue *url.Error
	if errors.As(last, &ue) {
		last = ue.Err
	}
	if last == nil || errors.Is(last, context.Canceled) || errors.Is(last, errStalled) {
		return fmt.Errorf("%w (gave up after %s)", errStalled, stallBudget)
	}
	return fmt.Errorf("%w: %v (gave up after %s)", errUnreachable, last, stallBudget)
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
	lastEnd := meta.Segments[i].End
	mu.Unlock()
	reconnects := 0
	lastProgress := time.Now()
	var lastErr error

	for {
		attemptCtx, cancel := context.WithCancel(ctx)
		tracker.begin(w, i, time.Now(), cancel)

		mu.Lock()
		local := meta.Segments[i]
		mu.Unlock()
		if local.End != lastEnd {
			// A steal shortened the range under us: the bytes are safe, and
			// this is progress, not another failed attempt.
			lastEnd = local.End
			reconnects = 0
		}
		add := func(n int64) {
			local.Done += n
			tracker.add(w, n)
			mu.Lock()
			if l := meta.Segments[i].length(); l >= 0 && local.Done > l {
				// Written past the shortened end before the cancel landed;
				// the tail is the thief's to account for.
				local.Done = l
			}
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
		// Only a file that has to restart, or a request the server
		// rejected outright, ends this segment here. Everything else -- our
		// watchdog cutting a silent connection, a reset, a TLS handshake
		// that never finished -- is the network having a bad minute, and
		// stallBudget below decides when that minute has lasted long
		// enough.
		//
		// Returning any other error killed the whole download the moment
		// four reconnects in a row failed: over a hotspot a burst of TLS
		// handshake timeouts is what four seconds of congestion looks
		// like, and the task died a fifth of the way into the file.
		if errors.Is(err, errRestart) || errors.Is(err, errPermanent) {
			return err
		}
		var he *httpError
		if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 && he.Status != http.StatusTooManyRequests {
			return err
		}
		lastErr = err
		mu.Lock()
		done := meta.Segments[i].Done
		mu.Unlock()
		if done > lastDone {
			reconnects = 0
			lastProgress = time.Now()
		} else {
			reconnects++
		}
		lastDone = done
		if time.Since(lastProgress) > stallBudget {
			return stallFailure(lastErr)
		}
		// Back off further after every attempt that brought nothing, so a
		// server that stopped answering is not hammered while the network
		// sorts itself out.
		delay := 150 * time.Millisecond
		if reconnects > 0 {
			delay = min(150*time.Millisecond<<min(reconnects, 5), 5*time.Second)
		}
		// Spread the retries out. Thirty-two workers reopening their TLS
		// connection on the same tick is what makes a thin link time the
		// handshakes out in the first place.
		delay += rand.N(delay / 2)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}
