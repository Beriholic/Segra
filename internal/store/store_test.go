package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsMigratesOldConnections(t *testing.T) {
	dir := t.TempDir()
	old := `{"downloadDir":"/tmp","connections":8,"maxConcurrent":3,"apiPort":47823}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadSettings(DefaultSettings("/tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Connections != 32 {
		t.Fatalf("connections = %d, want 32", got.Connections)
	}
	if got.Version != settingsVersion {
		t.Fatalf("version = %d, want %d", got.Version, settingsVersion)
	}
}

func TestDefaultSettingsClipboardFillIsOptIn(t *testing.T) {
	if DefaultSettings("/tmp").ClipboardFill {
		t.Fatal("clipboard fill must be off on a first run")
	}
}

func TestLoadSettingsKeepsCustomConnections(t *testing.T) {
	dir := t.TempDir()
	old := `{"downloadDir":"/tmp","connections":4,"maxConcurrent":3,"apiPort":47823}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(dir)
	got, err := s.LoadSettings(DefaultSettings("/tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Connections != 4 {
		t.Fatalf("connections = %d, want 4", got.Connections)
	}
}
