package download

import (
	"context"
	"os"
	"slices"
	"time"
)

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
	// The queue may start it before this returns, so the copy has to be
	// taken under the lock the pump writes through.
	snap := *t
	m.mu.Unlock()
	m.emit(EventCreated, &snap)
	m.changed()
	m.wake()
	return snap, nil
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
		// unwind, and a queued download should not wait for it. The task
		// itself does wait, though, so that the worker that is still
		// renaming its .part file cannot meet a second one doing the same.
		delete(m.running, id)
		m.draining[id] = true
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
	t.Interrupted = false
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
				m.draining[t.ID] = true
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
			t.Interrupted = false
			m.queue = append(m.queue, t.ID)
		}
	}
	m.mu.Unlock()
	m.changed()
	m.wake()
}

// ResumeInterrupted queues the tasks that were on their way when the app last
// quit, and only those: a task the user paused stays paused across launches.
func (m *Manager) ResumeInterrupted() {
	m.mu.Lock()
	for _, t := range m.tasks {
		if t.Status != StatusPaused || !t.Interrupted {
			continue
		}
		t.Status = StatusQueued
		t.Error = ""
		t.Interrupted = false
		m.queue = append(m.queue, t.ID)
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

// ClearStatus removes the tasks in one of the given statuses, optionally
// deleting their files, and reports how many records went. Removing a record
// is not deleting the file unless deleteFiles says so, which is why the
// history's tidying keeps to the records.
func (m *Manager) ClearStatus(deleteFiles bool, statuses ...Status) int {
	m.mu.Lock()
	var ids []string
	for _, id := range m.order {
		if t := m.tasks[id]; t != nil && slices.Contains(statuses, t.Status) {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Remove(id, deleteFiles)
	}
	return len(ids)
}

func (m *Manager) removeFromQueue(id string) {
	for i, v := range m.queue {
		if v == id {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			return
		}
	}
}
