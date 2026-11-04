package download

import (
	"context"
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

func (m *Manager) progress(id string, downloaded, speed int64) {
	m.mu.Lock()
	if t := m.tasks[id]; t != nil {
		t.Downloaded = downloaded
		t.Speed = speed
	}
	m.mu.Unlock()
	m.changed()
}
