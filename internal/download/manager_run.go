package download

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

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
