// Package download implements Segra's segmented HTTP download engine.
package download

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Status is the lifecycle state of a task.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusPaused      Status = "paused"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

// Task is one download. The manager owns the authoritative copy; List and
// Get return snapshots.
type Task struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	FinalURL     string    `json:"finalUrl,omitempty"`
	FileName     string    `json:"fileName"`
	Dir          string    `json:"dir"`
	Size         int64     `json:"size"` // -1 when the server does not say
	Downloaded   int64     `json:"downloaded"`
	Status       Status    `json:"status"`
	Error        string    `json:"error,omitempty"`
	Connections  int       `json:"connections"`
	Resumable    bool      `json:"resumable"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	ContentType  string    `json:"contentType,omitempty"`
	Referer      string    `json:"referer,omitempty"`
	UserAgent    string    `json:"userAgent,omitempty"`
	Cookie       string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	StartedAt    time.Time `json:"startedAt,omitempty"`
	CompletedAt  time.Time `json:"completedAt,omitempty"`

	// Speed is bytes per second, filled in by the manager while running.
	Speed int64 `json:"-"`

	// Segments is the live progress of the byte ranges, one per block the
	// panel draws. The manager rewrites the slice on every tick, so a
	// snapshot never sees it change under it. The .part metadata, not this
	// field, is what survives a restart.
	Segments []SegmentProgress `json:"-"`
}

// SegmentProgress is how much of one byte range is on disk.
type SegmentProgress struct {
	Start int64 `json:"start"`
	Done  int64 `json:"done"`
	Total int64 `json:"total"` // -1 when the server gave no length
}

// Path is the final path of the task's file.
func (t Task) Path() string { return filepath.Join(t.Dir, t.FileName) }

// Percent is the completed fraction in [0, 1], or 0 when the size is not
// known yet.
func (t Task) Percent() float64 {
	if t.Size <= 0 {
		return 0
	}
	p := float64(t.Downloaded) / float64(t.Size)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// Remaining is the number of bytes left, or -1 when unknown.
func (t Task) Remaining() int64 {
	if t.Size < 0 {
		return -1
	}
	if r := t.Size - t.Downloaded; r > 0 {
		return r
	}
	return 0
}

// Active reports whether the task holds a download slot or waits for one.
func (s Status) Active() bool {
	return s == StatusQueued || s == StatusDownloading
}

// Complete reports whether the task is done.
func (s Status) Complete() bool { return s == StatusCompleted }

// CreateRequest is what a caller asks the manager to download.
type CreateRequest struct {
	URL         string `json:"url"`
	FileName    string `json:"fileName,omitempty"`
	Dir         string `json:"dir,omitempty"`
	Referer     string `json:"referer,omitempty"`
	UserAgent   string `json:"userAgent,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	Connections int    `json:"connections,omitempty"`
}

// Config is the manager's runtime configuration.
type Config struct {
	// Dir is the default download directory.
	Dir string
	// Connections is the default number of connections per task.
	Connections int
	// MaxConcurrent is the number of tasks that download at once.
	MaxConcurrent int
}

func (c Config) withDefaults() Config {
	if c.Connections <= 0 {
		c.Connections = 32
	}
	if c.Connections > 64 {
		c.Connections = 64
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 3
	}
	if c.MaxConcurrent > 16 {
		c.MaxConcurrent = 16
	}
	return c
}

// EventKind tells what happened to a task.
type EventKind string

const (
	EventCreated   EventKind = "created"
	EventCompleted EventKind = "completed"
	EventFailed    EventKind = "failed"
)

// Event is delivered to the manager's event listener.
type Event struct {
	Kind EventKind
	Task Task
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func normalizeRequest(req CreateRequest, cfg Config) (CreateRequest, error) {
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return req, fmt.Errorf("URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return req, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return req, fmt.Errorf("only http and https URLs are supported")
	}
	if u.Host == "" {
		return req, fmt.Errorf("invalid URL: missing host")
	}
	req.URL = u.String()
	if strings.TrimSpace(req.Dir) == "" {
		req.Dir = cfg.Dir
	}
	if req.Connections <= 0 {
		req.Connections = cfg.Connections
	}
	if req.Connections > 64 {
		req.Connections = 64
	}
	return req, nil
}
