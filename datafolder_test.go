package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFrom(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "APOD")
	d := &dataFolder{dir: filepath.Join(root, "Nightglass")}

	// Nothing to migrate: no error, nothing created.
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.dir); err == nil {
		t.Fatal("created new dir with nothing to migrate")
	}

	// Old folder moves, settings survive, log is renamed.
	os.Mkdir(oldDir, 0o755)
	if err := (&dataFolder{dir: oldDir}).saveSettings(settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(oldDir, "apod.log"), []byte("old"), 0o644)
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if got := d.loadSettings(); got.BarStyle != BarsCrisp {
		t.Fatalf("migrated style = %q, want crisp", got.BarStyle)
	}
	if _, err := os.Stat(d.logPath()); err != nil {
		t.Fatalf("log not renamed: %v", err)
	}
	if _, err := os.Stat(oldDir); err == nil {
		t.Fatal("old dir still exists")
	}

	// New folder already exists: an old one reappearing is left alone.
	os.Mkdir(oldDir, 0o755)
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Fatal("touched old dir although new dir existed")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	d := &dataFolder{dir: t.TempDir()}
	if got := d.loadSettings(); got.BarStyle != BarsSoft {
		t.Fatalf("default style = %q, want soft", got.BarStyle)
	}
	if err := d.saveSettings(settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	if got := d.loadSettings(); got.BarStyle != BarsCrisp {
		t.Fatalf("reloaded style = %q, want crisp", got.BarStyle)
	}
}

func TestWriteAtomicFailureLeavesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.bin")
	boom := errors.New("boom")
	err := writeAtomic(p, func(w io.Writer) error {
		w.Write([]byte("partial"))
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	for _, f := range []string{p, p + ".part"} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s left behind after a failed write", filepath.Base(f))
		}
	}
}
