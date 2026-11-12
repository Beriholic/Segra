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

// A settings file from before update checks existed still checks for
// updates: the defaults fill what the file does not name.
func TestCheckUpdatesDefaultsOn(t *testing.T) {
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
	if !got.CheckUpdates {
		t.Fatal("update checks are off for a settings file without the field")
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

func TestSettingsKeepTheAppearance(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v := DefaultSettings("/tmp")
	v.Theme = ThemeDark
	if err := s.SaveSettings(v); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadSettings(DefaultSettings("/tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != ThemeDark {
		t.Fatalf("theme = %q, want %q", got.Theme, ThemeDark)
	}
}

// A theme a later build wrote, or a hand-edited file, is not an appearance
// the app can draw: the desktop's is the one that cannot be wrong.
func TestUnknownAppearanceFallsBackToTheSystem(t *testing.T) {
	dir := t.TempDir()
	old := `{"downloadDir":"/tmp","theme":"solarized"}`
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
	if got.Theme != ThemeSystem {
		t.Fatalf("theme = %q, want the system's", got.Theme)
	}
}

// A settings file from before the history kept itself still gets a tidy
// list: the defaults fill the fields it does not name.
func TestHistoryDefaultsOnForAnOlderSettingsFile(t *testing.T) {
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
	if !got.HistoryAutoClean {
		t.Fatal("the history is kept whole for a settings file without the field")
	}
	if got.HistoryDays != 30 || got.HistoryLimit != 200 {
		t.Fatalf("history = %d days, %d records, want 30 and 200", got.HistoryDays, got.HistoryLimit)
	}
}

// Zero is a choice the user made, not a field the file forgot: a file that
// says 0 keeps every record.
func TestHistoryLimitsKeepTheirZero(t *testing.T) {
	dir := t.TempDir()
	old := `{"downloadDir":"/tmp","historyDays":0,"historyLimit":0}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(dir)
	got, err := s.LoadSettings(DefaultSettings("/tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if got.HistoryDays != 0 || got.HistoryLimit != 0 {
		t.Fatalf("history = %d days, %d records, want no limits", got.HistoryDays, got.HistoryLimit)
	}
}
