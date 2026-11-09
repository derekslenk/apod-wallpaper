package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are user choices made from the tray menu, kept in settings.json.
type settings struct {
	BarStyle BarStyle `json:"bar_style"`
}

const logName = "nightglass.log"

// migrateDataDir moves the data folder used before the rename to Nightglass
// (%LOCALAPPDATA%\APOD) to its new place, keeping settings and downloads.
// It does nothing if the new folder already exists or the old one doesn't.
func migrateDataDir(oldDir, newDir string) error {
	if _, err := os.Stat(newDir); err == nil {
		return nil
	}
	if _, err := os.Stat(oldDir); err != nil {
		return nil
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}
	os.Rename(filepath.Join(newDir, "apod.log"), filepath.Join(newDir, logName))
	return nil
}

func loadSettings(dir string) settings {
	s := settings{BarStyle: BarsSoft}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		var raw struct {
			BarStyle string `json:"bar_style"`
		}
		if json.Unmarshal(b, &raw) == nil {
			s.BarStyle = parseBarStyle(raw.BarStyle)
		}
	}
	return s
}

func saveSettings(dir string, s settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "settings.json.part")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "settings.json"))
}
