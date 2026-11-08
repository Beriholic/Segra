package download

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync"
)

// Manager owns all tasks, the queue and the workers.
type Manager struct {
	mu      sync.Mutex
	tasks   map[string]*Task
	order   []string
	queue   []string
	running map[string]context.CancelFunc
	// draining holds the tasks whose cancelled worker has not returned yet.
	// They keep no slot in running, so that a queued download goes ahead,
	// but they cannot start again until the old worker is off their file.
	draining map[string]bool
	cfg      Config
	client   *http.Client
	notify   func()
	onEvent  func(Event)

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
		tasks:    make(map[string]*Task),
		running:  make(map[string]context.CancelFunc),
		draining: make(map[string]bool),
		cfg:      cfg.withDefaults(),
		client:   separateConnections(client),
		notify:   notify,
		wakeCh:   make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
	go m.loop()
	return m
}

// separateConnections returns a copy of c that speaks HTTP/1.1, so that each
// worker's range request gets its own TCP connection.
//
// A segmented download is only fast because every worker owns a connection
// with its own congestion window. Over HTTP/2 all the range requests are
// streams of a single connection sharing one flow-control window, and a CDN
// that stops returning window updates blocks every stream in turn: each one
// waits in writeRequest for room to send, with no body yet, so no timeout
// fires and the download sits at zero bytes for good. That is what a 32
// connection download of an Aliyun OSS object behind EdgeOne did.
//
// The caller's transport is left alone, since MyGo shares it with the
// updater and the fetch plugin; the copy carries its proxy and pool
// settings, and TLS still negotiates h2 for anything else using it.
func separateConnections(c *http.Client) *http.Client {
	var tr *http.Transport
	switch t := c.Transport.(type) {
	case *http.Transport:
		tr = t.Clone()
	case nil:
		tr = http.DefaultTransport.(*http.Transport).Clone()
	default:
		// Some other round tripper (a proxy wrapper, a recorder): leave it
		// as it is. The watchdogs keep a stuck request from hanging.
		return c
	}
	tr.ForceAttemptHTTP2 = false
	// ForceAttemptHTTP2 only stops the transport from adding h2 itself; an
	// empty TLS protocol map also refuses the server's ALPN offer.
	tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	// Turning h2 off leaves Go advertising no ALPN at all, and a network or
	// CDN that expects one drops the handshake: github.com closed the
	// connection with EOF for exactly this. Say what the connection speaks,
	// the way curl does for --http1.1.
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	} else {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	}
	tr.TLSClientConfig.NextProtos = []string{"http/1.1"}
	out := *c
	out.Transport = tr
	return &out
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
// paused so the user can resume them, and as interrupted, so that
// ResumeInterrupted picks them up and leaves the tasks the user paused alone.
func (m *Manager) Load(tasks []Task) {
	// Status first, then the metadata read: only a paused or failed task has
	// a .part file to read block progress from.
	for i := range tasks {
		switch tasks[i].Status {
		case StatusDownloading, StatusQueued:
			tasks[i].Status = StatusPaused
			tasks[i].Interrupted = true
		default:
			tasks[i].Interrupted = false
		}
		restoreSegments(&tasks[i])
	}

	m.mu.Lock()
	for i := range tasks {
		t := tasks[i]
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

func (m *Manager) progress(id string, downloaded, speed int64, segments []SegmentProgress) {
	m.mu.Lock()
	if t := m.tasks[id]; t != nil {
		t.Downloaded = downloaded
		t.Speed = speed
		t.Segments = segments
	}
	m.mu.Unlock()
	m.changed()
}
