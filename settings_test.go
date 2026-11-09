package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateDataDir(t *testing.T) {
	root := t.TempDir()
	oldDir, newDir := filepath.Join(root, "APOD"), filepath.Join(root, "Nightglass")

	// Nothing to migrate: no error, nothing created.
	if err := migrateDataDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newDir); err == nil {
		t.Fatal("created new dir with nothing to migrate")
	}

	// Old folder moves, settings survive, log is renamed.
	os.Mkdir(oldDir, 0o755)
	if err := saveSettings(oldDir, settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(oldDir, "apod.log"), []byte("old"), 0o644)
	if err := migrateDataDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(newDir); got.BarStyle != BarsCrisp {
		t.Fatalf("migrated style = %q, want crisp", got.BarStyle)
	}
	if _, err := os.Stat(filepath.Join(newDir, logName)); err != nil {
		t.Fatalf("log not renamed: %v", err)
	}
	if _, err := os.Stat(oldDir); err == nil {
		t.Fatal("old dir still exists")
	}

	// New folder already exists: an old one reappearing is left alone.
	os.Mkdir(oldDir, 0o755)
	if err := migrateDataDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Fatal("touched old dir although new dir existed")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := loadSettings(dir); got.BarStyle != BarsSoft {
		t.Fatalf("default style = %q, want soft", got.BarStyle)
	}
	if err := saveSettings(dir, settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(dir); got.BarStyle != BarsCrisp {
		t.Fatalf("reloaded style = %q, want crisp", got.BarStyle)
	}
}
