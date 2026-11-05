package download

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Health tuning, modelled on Surge's concurrent downloader: a connection
// that stops delivering bytes, or that runs far below the mean speed of its
// peers, is dropped and re-opened. On CDNs that throttle individual
// connections this is what keeps aggregate throughput high.
//
// Variables so that tests can shrink them; nothing else changes them.
var (
	healthInterval = 1 * time.Second
	healthGrace    = 4 * time.Second
	stallAfter     = 3 * time.Second
	maxReconnects  = 8
)

const slowRatio = 0.30

// workerHealth is the live state of one connection while it streams a
// segment. All fields read by the health loop are atomic.
type workerHealth struct {
	active     atomic.Bool
	seg        atomic.Int32
	bytes      atomic.Int64 // total bytes written by this worker
	started    atomic.Int64 // unix nanos of the current attempt
	lastActive atomic.Int64 // unix nanos of the last byte written

	cancelMu sync.Mutex
	cancel   context.CancelFunc

	// The health loop owns these.
	lastBytes int64
	lastTick  time.Time
	speed     float64
}

// healthTracker watches all workers of one download.
type healthTracker struct {
	workers []*workerHealth
}

func newHealthTracker(n int) *healthTracker {
	h := &healthTracker{workers: make([]*workerHealth, n)}
	for i := range h.workers {
		h.workers[i] = &workerHealth{}
		h.workers[i].seg.Store(-1)
	}
	return h
}

func (h *healthTracker) begin(i, seg int, started time.Time, cancel context.CancelFunc) {
	w := h.workers[i]
	w.cancelMu.Lock()
	w.cancel = cancel
	w.cancelMu.Unlock()
	w.seg.Store(int32(seg))
	w.started.Store(started.UnixNano())
	w.lastActive.Store(started.UnixNano())
	w.active.Store(true)
}

func (h *healthTracker) add(i int, n int64) {
	w := h.workers[i]
	if n <= 0 {
		return
	}
	w.bytes.Add(n)
	w.lastActive.Store(time.Now().UnixNano())
}

func (h *healthTracker) end(i int) {
	w := h.workers[i]
	w.active.Store(false)
	w.cancelMu.Lock()
	w.cancel = nil
	w.cancelMu.Unlock()
}

// watch runs until ctx is done. Every tick it measures each worker's speed,
// then cancels the ones that stalled or fell well below the mean. The worker
// reacts by re-opening its segment, which starts a fresh connection.
func (h *healthTracker) watch(ctx context.Context) {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.check(now)
		}
	}
}

func (h *healthTracker) check(now time.Time) {
	var mean float64
	var counted int
	type slow struct{ i int }

	for _, w := range h.workers {
		if !w.active.Load() {
			w.lastTick = time.Time{}
			w.speed = 0
			continue
		}
		b := w.bytes.Load()
		if w.lastTick.IsZero() {
			w.lastBytes = b
			w.lastTick = now
			w.speed = 0
			continue
		}
		dt := now.Sub(w.lastTick).Seconds()
		if dt > 0 {
			w.speed = float64(b-w.lastBytes) / dt
		} else {
			w.speed = 0
		}
		w.lastBytes = b
		w.lastTick = now
		if w.speed > 0 {
			mean += w.speed
			counted++
		}
	}
	if counted > 0 {
		mean /= float64(counted)
	}

	var drop []slow
	for i, w := range h.workers {
		if !w.active.Load() {
			continue
		}
		started := time.Unix(0, w.started.Load())
		age := now.Sub(started)
		if age < healthGrace {
			continue
		}
		last := time.Unix(0, w.lastActive.Load())
		if now.Sub(last) >= stallAfter {
			drop = append(drop, slow{i: i})
			continue
		}
		if mean > 0 && w.speed > 0 && w.speed < slowRatio*mean {
			drop = append(drop, slow{i: i})
		}
	}

	for _, d := range drop {
		w := h.workers[d.i]
		w.cancelMu.Lock()
		cancel := w.cancel
		// Do not cancel twice: a reconnecting worker clears the slot.
		w.cancel = nil
		w.cancelMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}
