// Package store keeps Segra's settings and task list on disk.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/beriholic/segra/internal/download"
)

const settingsVersion = 1

// The appearances the settings offer: the desktop's, or one of the two the
// app draws itself.
const (
	ThemeSystem = "" // follow the desktop
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Settings are the user's preferences.
type Settings struct {
	Version       int    `json:"version"`
	DownloadDir   string `json:"downloadDir"`
	Connections   int    `json:"connections"`
	MaxConcurrent int    `json:"maxConcurrent"`
	APIPort       int    `json:"apiPort"`
	APIToken      string `json:"apiToken"`
	EnableAPI     bool   `json:"enableAPI"`
	Language      string `json:"language"` // "" follows the system
	Theme         string `json:"theme"`    // "" follows the system
	Notifications bool   `json:"notifications"`
	AutoResume    bool   `json:"autoResume"`
	ClipboardFill bool   `json:"clipboardFill"`
	OpenAtLogin   bool   `json:"openAtLogin"`
	CloseToTray   bool   `json:"closeToTray"`
	HideDock      bool   `json:"hideDock"` // macOS: no Dock icon once the window closes

	// CheckUpdates asks GitHub for a newer release when the app starts.
	// UpdateCheckedAt is when a check last found nothing newer, in Unix
	// seconds, so that a relaunch within a day does not ask again.
	CheckUpdates    bool  `json:"checkUpdates"`
	UpdateCheckedAt int64 `json:"updateCheckedAt,omitempty"`

	// History keeps the list from growing without end. HistoryAutoClean drops
	// finished records as the app runs: those that ended more than
	// HistoryDays ago, and all but the HistoryLimit newest of them, 0 in
	// either meaning no limit there. Only records go; the files a download
	// wrote are never touched, and a download still on its way is not
	// history.
	HistoryAutoClean bool `json:"historyAutoClean"`
	HistoryDays      int  `json:"historyDays"`
	HistoryLimit     int  `json:"historyLimit"`
}

// DefaultSettings returns settings for a first run.
func DefaultSettings(downloadDir string) Settings {
	return Settings{
		Version:       settingsVersion,
		DownloadDir:   downloadDir,
		Connections:   32,
		MaxConcurrent: 3,
		APIPort:       47823,
		EnableAPI:     false,
		Language:      "",
		Notifications: true,
		AutoResume:    true,
		ClipboardFill: false,
		OpenAtLogin:   false,
		CloseToTray:   false,
		HideDock:      false,
		CheckUpdates:  true,
		// A month and a couple of hundred rows: enough to find last week's
		// download, few enough that the list stays a list.
		HistoryAutoClean: true,
		HistoryDays:      30,
		HistoryLimit:     200,
	}
}

// Normalize clamps values that came from disk or a form.
func (s *Settings) Normalize() {
	if s.Connections < 1 {
		s.Connections = 1
	}
	if s.Connections > 64 {
		s.Connections = 64
	}
	if s.MaxConcurrent < 1 {
		s.MaxConcurrent = 1
	}
	if s.MaxConcurrent > 8 {
		s.MaxConcurrent = 8
	}
	if s.APIPort <= 0 || s.APIPort > 65535 {
		s.APIPort = 47823
	}
	if s.DownloadDir == "" {
		s.DownloadDir = "."
	}
	if s.HistoryDays < 0 {
		s.HistoryDays = 0
	}
	if s.HistoryDays > 3650 {
		s.HistoryDays = 3650
	}
	if s.HistoryLimit < 0 {
		s.HistoryLimit = 0
	}
	if s.HistoryLimit > 100000 {
		s.HistoryLimit = 100000
	}
	switch s.Theme {
	case ThemeLight, ThemeDark:
	default:
		// A value from a newer build, or a hand-edited file: the desktop's
		// appearance is the one that cannot be wrong.
		s.Theme = ThemeSystem
	}
}

// Store reads and writes files in the app's data directory.
type Store struct {
	dir string
}

// Open creates the data directory when needed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

// SaveTasks writes the task list atomically.
func (s *Store) SaveTasks(tasks []download.Task) error {
	if tasks == nil {
		tasks = []download.Task{}
	}
	return writeJSON(s.path("tasks.json"), tasks, 0o600)
}

// LoadTasks reads the task list. A corrupt file is moved aside and an
// empty list is returned.
func (s *Store) LoadTasks() ([]download.Task, error) {
	var tasks []download.Task
	if err := readJSON(s.path("tasks.json"), &tasks); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		_ = os.Rename(s.path("tasks.json"), s.path("tasks.json.corrupt"))
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	return tasks, nil
}

// SaveSettings writes the settings atomically.
func (s *Store) SaveSettings(v Settings) error {
	v.Version = settingsVersion
	return writeJSON(s.path("settings.json"), v, 0o600)
}

// LoadSettings reads the settings, filling defaults for missing fields.
func (s *Store) LoadSettings(defaults Settings) (Settings, error) {
	v := defaults
	v.Version = 0 // a file without a version predates settingsVersion
	if err := readJSON(s.path("settings.json"), &v); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return v, nil
		}
		return v, fmt.Errorf("read settings: %w", err)
	}
	v.Normalize()
	if v.Version < settingsVersion {
		// 0.1.0 shipped with 8 connections; raise it once for existing users.
		if v.Connections == 8 {
			v.Connections = 32
		}
		v.Version = settingsVersion
	}
	return v, nil
}

func writeJSON(path string, v any, mode os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".segra-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// BackupTasks copies the task list aside, for diagnostics.
func (s *Store) BackupTasks() error {
	data, err := os.ReadFile(s.path("tasks.json"))
	if err != nil {
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	return os.WriteFile(s.path("tasks-"+stamp+".json.bak"), data, 0o600)
}
