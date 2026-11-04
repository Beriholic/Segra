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

// Settings are the user's preferences.
type Settings struct {
	Version       int    `json:"version"`
	DownloadDir   string `json:"downloadDir"`
	Connections   int    `json:"connections"`
	MaxConcurrent int    `json:"maxConcurrent"`
	APIPort       int    `json:"apiPort"`
	APIToken      string `json:"apiToken"`
	Language      string `json:"language"` // "" follows the system
	Notifications bool   `json:"notifications"`
	AutoResume    bool   `json:"autoResume"`
	OpenAtLogin   bool   `json:"openAtLogin"`
	CloseToTray   bool   `json:"closeToTray"`
}

// DefaultSettings returns settings for a first run.
func DefaultSettings(downloadDir string) Settings {
	return Settings{
		Version:       settingsVersion,
		DownloadDir:   downloadDir,
		Connections:   32,
		MaxConcurrent: 3,
		APIPort:       47823,
		Language:      "",
		Notifications: true,
		AutoResume:    true,
		OpenAtLogin:   false,
		CloseToTray:   false,
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
