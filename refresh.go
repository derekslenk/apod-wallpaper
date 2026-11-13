package main

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	keepImages = 7
	feedURL    = "https://science.nasa.gov/feed/apod-basic/"
)

// refresher runs one Refresh: fetch the feed, pick the Latest and Previous
// pictures, render one wallpaper per monitor, apply them, prune old downloads.
// It is platform-neutral; the desktop seam is the only OS-specific part.
type refresher struct {
	dir     string
	feedURL string
	client  *http.Client
	// desktop lists the monitors, asks prepare for a wallpaper file per monitor
	// ID, and applies them. Windows: setWallpapers. Tests: a closure.
	desktop func(prepare func([]Monitor) (map[string]string, error)) error
}

// refresh runs one Refresh and returns the Latest picture (the one shown on
// the Main monitor). Not safe for concurrent calls: it writes to dir.
func (r *refresher) refresh(ctx context.Context, style BarStyle) (Entry, error) {
	entries, err := r.fetchEntries(ctx)
	if err != nil {
		return Entry{}, err
	}
	imgs := pickImages(entries, 2)
	if len(imgs) == 0 {
		return Entry{}, fmt.Errorf("no image entries in the APOD feed")
	}

	var paths []string
	inUse := map[string]bool{}
	for _, e := range imgs {
		p := filepath.Join(r.dir, e.localFileName())
		if _, err := os.Stat(p); err != nil {
			if err := r.download(ctx, e.Image, p); err != nil {
				return Entry{}, fmt.Errorf("download %s: %w", e.Date, err)
			}
			log.Printf("downloaded %s %q", e.Date, e.Title)
		}
		paths = append(paths, p)
		inUse[filepath.Base(p)] = true
	}

	if err := r.desktop(func(mons []Monitor) (map[string]string, error) {
		return r.renderForMonitors(mons, paths, style)
	}); err != nil {
		return Entry{}, err
	}
	log.Printf("wallpaper set: today=%s", imgs[0].Date)

	if names, err := os.ReadDir(r.dir); err == nil {
		var ns []string
		for _, n := range names {
			ns = append(ns, n.Name())
		}
		for _, n := range filesToDelete(ns, keepImages, inUse) {
			os.Remove(filepath.Join(r.dir, n))
		}
	}
	return imgs[0], nil
}

// renderForMonitors builds a centered, bar-matched wallpaper for each monitor
// at its exact resolution, in <dir>\rendered\. Renders are cached by
// source+size+style; stale ones are removed.
func (r *refresher) renderForMonitors(mons []Monitor, paths []string, style BarStyle) (map[string]string, error) {
	rdir := filepath.Join(r.dir, "rendered")
	if err := os.MkdirAll(rdir, 0o755); err != nil {
		return nil, err
	}
	assigned := assignWallpapers(mons, paths)
	out := map[string]string{}
	keep := map[string]bool{}
	for _, m := range mons {
		srcPath := assigned[m.ID]
		base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
		dst := filepath.Join(rdir, fmt.Sprintf("%s_%dx%d_v4_%s.png", base, m.Width, m.Height, style))
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
	tmp := dst + ".part"
	o, err := os.Create(tmp)
	if err != nil {
		return err
	}
	// PNG so the bars stay a perfectly flat color (no JPEG blocking).
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(o, compose(img, w, h, style)); err != nil {
		o.Close()
		os.Remove(tmp)
		return err
	}
	if err := o.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func (r *refresher) fetchEntries(ctx context.Context) ([]Entry, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", r.feedURL, nil)
	req.Header.Set("User-Agent", "nightglass")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("APOD feed: HTTP %s", resp.Status)
	}
	return parseFeed(resp.Body)
}

func (r *refresher) download(ctx context.Context, u, dest string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "nightglass")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}
