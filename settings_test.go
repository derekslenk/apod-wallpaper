package main

import "testing"

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
