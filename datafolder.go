package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	keepImages    = 7
	renderVersion = 4 // bump when compose output changes, so cached renders are redone
	logName       = "nightglass.log"
)

// dataFolder is the Data folder (%LOCALAPPDATA%\Nightglass). It owns every
// file name in there: pictures as YYYY-MM-DD.ext in the root, renders in
// rendered\, settings.json and the log. Callers never build a path themselves.
type dataFolder struct{ dir string }

func (d *dataFolder) logPath() string { return filepath.Join(d.dir, logName) }

// settings are user choices made from the tray menu, kept in settings.json.
type settings struct {
	BarStyle BarStyle `json:"bar_style"`
}

// migrateFrom moves the folder used before the rename to Nightglass
// (%LOCALAPPDATA%\APOD) to this one, keeping settings and downloads.
// It does nothing if this folder already exists or the old one doesn't.
func (d *dataFolder) migrateFrom(oldDir string) error {
	if _, err := os.Stat(d.dir); err == nil {
		return nil
	}
	if _, err := os.Stat(oldDir); err != nil {
		return nil
	}
	if err := os.Rename(oldDir, d.dir); err != nil {
		return err
	}
	os.Rename(filepath.Join(d.dir, "apod.log"), d.logPath())
	return nil
}

func (d *dataFolder) loadSettings() settings {
	s := settings{BarStyle: BarsSoft}
	if b, err := os.ReadFile(filepath.Join(d.dir, "settings.json")); err == nil {
		var raw struct {
			BarStyle string `json:"bar_style"`
		}
		if json.Unmarshal(b, &raw) == nil {
			s.BarStyle = parseBarStyle(raw.BarStyle)
		}
	}
	return s
}

func (d *dataFolder) saveSettings(s settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(d.dir, "settings.json"), func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	})
}

// localFileName is "YYYY-MM-DD.ext", using the image's own extension.
func (e Entry) localFileName() string {
	if e.isImage() {
		return e.Date + e.ext()
	}
	return e.Date + ".jpg"
}

// pictures makes sure each entry's picture is on disk, fetching the missing
// ones, and returns their paths in the entries' order.
func (d *dataFolder) pictures(ctx context.Context, entries []Entry, fetch func(ctx context.Context, url string, w io.Writer) error) ([]string, error) {
	var paths []string
	for _, e := range entries {
		p := filepath.Join(d.dir, e.localFileName())
		if _, err := os.Stat(p); err != nil {
			if err := writeAtomic(p, func(w io.Writer) error { return fetch(ctx, e.Image, w) }); err != nil {
				return nil, fmt.Errorf("download %s: %w", e.Date, err)
			}
			log.Printf("downloaded %s %q", e.Date, e.Title)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

var datedFile = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.[a-z]+$`)

// filesToDelete returns downloaded images beyond the newest `keep`,
// never including any file in inUse.
func filesToDelete(names []string, keep int, inUse map[string]bool) []string {
	var dated []string
	for _, n := range names {
		if datedFile.MatchString(n) {
			dated = append(dated, n)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dated)))
	var del []string
	for i, n := range dated {
		if i >= keep && !inUse[n] {
			del = append(del, n)
		}
	}
	return del
}

// prune deletes pictures beyond the newest keepImages, never one in inUse
// (picture paths as returned by pictures). Settings and the log are never
// dated, so they are never candidates.
func (d *dataFolder) prune(inUse []string) {
	keep := map[string]bool{}
	for _, p := range inUse {
		keep[filepath.Base(p)] = true
	}
	ents, err := os.ReadDir(d.dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	for _, n := range filesToDelete(names, keepImages, keep) {
		os.Remove(filepath.Join(d.dir, n))
	}
}

// renders builds a centered, bar-matched wallpaper for each monitor at its
// exact resolution from the picture assigned to it, in rendered\. Renders are
// cached by picture+size+style; stale ones are removed.
func (d *dataFolder) renders(assigned map[string]string, mons []Monitor, style BarStyle) (map[string]string, error) {
	rdir := filepath.Join(d.dir, "rendered")
	if err := os.MkdirAll(rdir, 0o755); err != nil {
		return nil, err
	}
	out := map[string]string{}
	keep := map[string]bool{}
	for _, m := range mons {
		srcPath := assigned[m.ID]
		base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
		dst := filepath.Join(rdir, fmt.Sprintf("%s_%dx%d_v%d_%s.png", base, m.Width, m.Height, renderVersion, style))
		keep[filepath.Base(dst)] = true
		out[m.ID] = dst
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := renderFile(srcPath, dst, m.Width, m.Height, style); err != nil {
			return nil, fmt.Errorf("render %s for %dx%d: %w", filepath.Base(srcPath), m.Width, m.Height, err)
		}
	}
	if ents, err := os.ReadDir(rdir); err == nil {
		for _, e := range ents {
			if !keep[e.Name()] {
				os.Remove(filepath.Join(rdir, e.Name()))
			}
		}
	}
	return out, nil
}

func renderFile(src, dst string, w, h int, style BarStyle) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	// PNG so the bars stay a perfectly flat color (no JPEG blocking).
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return writeAtomic(dst, func(o io.Writer) error { return enc.Encode(o, compose(img, w, h, style)) })
}

// writeAtomic writes to path+".part" and renames it into place, so a reader
// never sees a half-written file and a failed write leaves nothing behind.
func writeAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
