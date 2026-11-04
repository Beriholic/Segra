package download

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const metaVersion = 1

// segmentState is one byte range of a partial download.
type segmentState struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`  // inclusive, -1 when unknown
	Done  int64 `json:"done"` // bytes written from Start
}

func (s segmentState) length() int64 {
	if s.End < 0 {
		return -1
	}
	return s.End - s.Start + 1
}

// taskMeta is persisted next to the .part file so a download can resume
// after a restart.
type taskMeta struct {
	Version     int            `json:"version"`
	TaskID      string         `json:"taskId"`
	URL         string         `json:"url"`
	FinalURL    string         `json:"finalUrl,omitempty"`
	ETag        string         `json:"etag,omitempty"`
	LastMod     string         `json:"lastModified,omitempty"`
	Size        int64          `json:"size"`
	Disposition string         `json:"disposition,omitempty"`
	ContentType string         `json:"contentType,omitempty"`
	Referer     string         `json:"referer,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
	Cookie      string         `json:"cookie,omitempty"`
	Single      bool           `json:"single"` // the server ignores Range
	Segments    []segmentState `json:"segments"`
}

func (m *taskMeta) downloaded() int64 {
	var n int64
	for _, s := range m.Segments {
		n += s.Done
	}
	return n
}

func (m *taskMeta) complete() bool {
	if m.Size < 0 {
		return false
	}
	return m.downloaded() >= m.Size
}

func readMeta(path string) (*taskMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m taskMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if m.Version != metaVersion {
		return nil, fmt.Errorf("metadata version %d is not supported", m.Version)
	}
	return &m, nil
}

func writeMeta(path string, m *taskMeta) error {
	m.Version = metaVersion
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o600)
}

func removeMeta(path string) {
	_ = os.Remove(path)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".segra-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// speedMeter estimates throughput from the total number of bytes written.
type speedMeter struct {
	lastBytes int64
	lastAt    time.Time
	rate      float64
}

func (s *speedMeter) update(total int64, now time.Time) float64 {
	if s.lastAt.IsZero() {
		s.lastAt, s.lastBytes = now, total
		return 0
	}
	elapsed := now.Sub(s.lastAt).Seconds()
	if elapsed < 0.2 {
		return s.rate
	}
	delta := float64(total - s.lastBytes)
	s.rate = delta / elapsed
	s.lastAt, s.lastBytes = now, total
	return s.rate
}
