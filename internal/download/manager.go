package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Manager owns all tasks, the queue and the workers.
type Manager struct {
	mu      sync.Mutex
	tasks   map[string]*Task
	order   []string
	queue   []string
	running map[string]context.CancelFunc
	cfg     Config
	client  *http.Client
	notify  func()
	onEvent func(Event)

	wakeCh   chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	closed   bool
}

// New starts a manager. client may be nil, for a client without a timeout
// that uses the system proxy.
func New(client *http.Client, cfg Config, notify func()) *Manager {
	if client == nil {
		client = &http.Client{}
	}
	m := &Manager{
		tasks:   make(map[string]*Task),
		running: make(map[string]context.CancelFunc),
		cfg:     cfg.withDefaults(),
		client:  client,
		notify:  notify,
		wakeCh:  make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
	go m.loop()
	return m
}

// SetOnEvent installs the listener for completed, failed and created tasks.
func (m *Manager) SetOnEvent(fn func(Event)) {
	m.mu.Lock()
	m.onEvent = fn
	m.mu.Unlock()
}

// SetConfig applies new defaults and wakes the queue.
func (m *Manager) SetConfig(cfg Config) {
	m.mu.Lock()
	m.cfg = cfg.withDefaults()
	m.mu.Unlock()
	m.wake()
}

// Config returns the current configuration.
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Load adds tasks read from disk. Tasks that were downloading are marked
// paused so the user can resume them.
func (m *Manager) Load(tasks []Task) {
	m.mu.Lock()
	for i := range tasks {
		t := tasks[i]
		switch t.Status {
		case StatusDownloading, StatusQueued:
			t.Status = StatusPaused
		}
		if t.Status == StatusFailed && t.Downloaded <= 0 {
			// keep the failure visible
		}
		if _, ok := m.tasks[t.ID]; ok {
			continue
		}
		cp := t
		m.tasks[t.ID] = &cp
		m.order = append(m.order, t.ID)
	}
	m.mu.Unlock()
	m.changed()
}

// Create validates a request and queues a task.
func (m *Manager) Create(req CreateRequest) (Task, error) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	req, err := normalizeRequest(req, cfg)
	if err != nil {
		return Task{}, err
	}
	if req.Dir == "" {
		req.Dir = cfg.Dir
	}
	if req.Dir == "" {
		req.Dir = "."
	}
	t := &Task{
		ID:          newID(),
		URL:         req.URL,
		FileName:    "",
		Dir:         req.Dir,
		Size:        -1,
		Status:      StatusQueued,
		Connections: req.Connections,
		Referer:     req.Referer,
		UserAgent:   req.UserAgent,
		Cookie:      req.Cookie,
		CreatedAt:   time.Now(),
	}
	if req.FileName != "" {
		t.FileName = Sanitize(req.FileName)
	}
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	m.queue = append(m.queue, t.ID)
	m.mu.Unlock()
	m.emit(EventCreated, t)
	m.changed()
	m.wake()
	return *t, nil
}

// List returns snapshots, newest first.
func (m *Manager) List() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		if t := m.tasks[m.order[i]]; t != nil {
			out = append(out, *t)
		}
	}
	return out
}

// Get returns a snapshot of one task.
func (m *Manager) Get(id string) (Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}

// ActiveCount is the number of tasks downloading now.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running)
}

// Pause stops a task, keeping its partial file for a later resume.
func (m *Manager) Pause(id string) {
	var cancel context.CancelFunc
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	switch t.Status {
	case StatusDownloading:
		t.Status = StatusPaused
		cancel = m.running[id]
		// Free the slot now: the cancelled worker may take a moment to
		// unwind, and a queued download should not wait for it.
		delete(m.running, id)
	case StatusQueued:
		t.Status = StatusPaused
		m.removeFromQueue(id)
	default:
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.changed()
}

// Resume queues a paused or failed task again.
func (m *Manager) Resume(id string) {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	if t.Status != StatusPaused && t.Status != StatusFailed {
		m.mu.Unlock()
		return
	}
	t.Status = StatusQueued
	t.Error = ""
	m.queue = append(m.queue, id)
	m.mu.Unlock()
	m.changed()
	m.wake()
}

// Retry is Resume.
func (m *Manager) Retry(id string) { m.Resume(id) }

// PauseAll pauses every queued or running task.
func (m *Manager) PauseAll() {
	m.mu.Lock()
	var cancels []context.CancelFunc
	for _, t := range m.tasks {
		switch t.Status {
		case StatusDownloading:
			t.Status = StatusPaused
			if c := m.running[t.ID]; c != nil {
				cancels = append(cancels, c)
				delete(m.running, t.ID)
			}
		case StatusQueued:
			t.Status = StatusPaused
		}
	}
	m.queue = nil
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	m.changed()
}

// ResumeAll queues every paused task.
func (m *Manager) ResumeAll() {
	m.mu.Lock()
	for _, t := range m.tasks {
		if t.Status == StatusPaused {
			t.Status = StatusQueued
			t.Error = ""
			m.queue = append(m.queue, t.ID)
		}
	}
	m.mu.Unlock()
	m.changed()
	m.wake()
}

// Remove deletes a task and, when deleteFiles is true, its files.
func (m *Manager) Remove(id string, deleteFiles bool) {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	if c := m.running[id]; c != nil {
		c()
	}
	delete(m.tasks, id)
	m.removeFromQueue(id)
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	snapshot := *t
	m.mu.Unlock()
	if deleteFiles {
		_ = os.Remove(snapshot.Path())
		_ = os.Remove(snapshot.PartPath())
		removeMeta(MetadataPath(snapshot.PartPath()))
	}
	m.changed()
}

// ClearCompleted removes completed tasks, optionally deleting their files.
func (m *Manager) ClearCompleted(deleteFiles bool) {
	m.mu.Lock()
	var ids []string
	for id, t := range m.tasks {
		if t.Status == StatusCompleted {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Remove(id, deleteFiles)
	}
}

// Close pauses the running tasks and waits for their workers.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	var cancels []context.CancelFunc
	for id, c := range m.running {
		if t := m.tasks[id]; t != nil {
			t.Status = StatusPaused
		}
		cancels = append(cancels, c)
	}
	m.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	m.stopOnce.Do(func() { close(m.stop) })
	m.wg.Wait()
}

func (m *Manager) removeFromQueue(id string) {
	for i, v := range m.queue {
		if v == id {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			return
		}
	}
}

func (m *Manager) loop() {
	for {
		select {
		case <-m.stop:
			return
		case <-m.wakeCh:
			m.pump()
		}
	}
}

func (m *Manager) wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

func (m *Manager) pump() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	started := 0
	for len(m.queue) > 0 && len(m.running) < m.cfg.MaxConcurrent {
		id := m.queue[0]
		m.queue = m.queue[1:]
		t := m.tasks[id]
		if t == nil || t.Status != StatusQueued {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Status = StatusDownloading
		t.StartedAt = time.Now()
		t.Error = ""
		m.running[id] = cancel
		m.wg.Add(1)
		go func(t *Task, ctx context.Context) {
			defer m.wg.Done()
			m.run(ctx, t)
		}(t, ctx)
		started++
	}
	m.mu.Unlock()
	if started > 0 {
		m.changed()
	}
}

func (m *Manager) run(ctx context.Context, t *Task) {
	defer func() {
		m.mu.Lock()
		delete(m.running, t.ID)
		m.mu.Unlock()
		m.changed()
		m.wake()
	}()

	err := m.execute(ctx, t)
	if err == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if errors.Is(err, errRestart) {
		_ = os.Remove(t.PartPath())
		removeMeta(MetadataPath(t.PartPath()))
		if err = m.execute(ctx, t); err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
	m.fail(t, err)
}

func (m *Manager) execute(ctx context.Context, t *Task) error {
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	meta, part, metaPath, err := m.prepare(ctx, t)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if meta.Size > 0 {
		_ = f.Truncate(meta.Size)
	}

	if err := m.transfer(ctx, t, meta, f, metaPath); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	size := meta.Size
	if size < 0 {
		size = st.Size()
	}
	if st.Size() != size {
		_ = f.Close()
		return fmt.Errorf("size mismatch: have %d bytes, want %d", st.Size(), size)
	}
	if err := f.Close(); err != nil {
		return err
	}

	final := t.Path()
	if _, err := os.Stat(final); err == nil {
		_ = os.Remove(final)
	}
	if err := os.Rename(part, final); err != nil {
		return err
	}
	removeMeta(metaPath)
	m.complete(t, size)
	return nil
}

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

func (m *Manager) complete(t *Task, size int64) {
	m.mu.Lock()
	t.Status = StatusCompleted
	t.Size = size
	t.Downloaded = size
	t.CompletedAt = time.Now()
	t.Speed = 0
	t.Error = ""
	snapshot := *t
	m.mu.Unlock()
	m.emit(EventCompleted, &snapshot)
	m.changed()
}

func (m *Manager) fail(t *Task, err error) {
	msg := "download failed"
	var he *httpError
	switch {
	case errors.As(err, &he):
		msg = he.Error()
	case errors.Is(err, context.Canceled):
		return
	default:
		msg = err.Error()
	}
	m.mu.Lock()
	t.Status = StatusFailed
	t.Error = msg
	t.Speed = 0
	snapshot := *t
	m.mu.Unlock()
	m.emit(EventFailed, &snapshot)
	m.changed()
}

func (m *Manager) progress(id string, downloaded, speed int64) {
	m.mu.Lock()
	if t := m.tasks[id]; t != nil {
		t.Downloaded = downloaded
		t.Speed = speed
	}
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) changed() {
	m.mu.Lock()
	fn := m.notify
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Manager) emit(kind EventKind, t *Task) {
	m.mu.Lock()
	fn := m.onEvent
	var snapshot Task
	if t != nil {
		snapshot = *t
	}
	m.mu.Unlock()
	if fn != nil {
		fn(Event{Kind: kind, Task: snapshot})
	}
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
		if !errors.Is(err, context.Canceled) {
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
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
