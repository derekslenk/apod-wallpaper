package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
)

const feedURL = "https://science.nasa.gov/feed/apod-basic/"

// refresher runs one Refresh: fetch the feed, pick the Latest and Previous
// pictures, render one wallpaper per monitor, apply them, prune old downloads.
// It is platform-neutral; the desktop seam is the only OS-specific part.
type refresher struct {
	folder  *dataFolder
	feedURL string
	client  *http.Client
	// desktop lists the monitors, asks prepare for a wallpaper file per monitor
	// ID, and applies them. Windows: setWallpapers. Tests: a closure.
	desktop func(prepare func([]Monitor) (map[string]string, error)) error
}

// refresh runs one Refresh and returns the Latest picture (the one shown on
// the Main monitor). Not safe for concurrent calls: it writes to the folder.
func (r *refresher) refresh(ctx context.Context, style BarStyle) (Entry, error) {
	entries, err := r.fetchEntries(ctx)
	if err != nil {
		return Entry{}, err
	}
	imgs := pickImages(entries, 2)
	if len(imgs) == 0 {
		return Entry{}, fmt.Errorf("no image entries in the APOD feed")
	}
	paths, err := r.folder.pictures(ctx, imgs, r.fetch)
	if err != nil {
		return Entry{}, err
	}
	if err := r.desktop(func(mons []Monitor) (map[string]string, error) {
		return r.folder.renders(assignWallpapers(mons, paths), mons, style)
	}); err != nil {
		return Entry{}, err
	}
	log.Printf("wallpaper set: today=%s", imgs[0].Date)
	r.folder.prune(paths)
	return imgs[0], nil
}

func (r *refresher) fetchEntries(ctx context.Context) ([]Entry, error) {
	resp, err := r.get(ctx, r.feedURL)
	if err != nil {
		return nil, fmt.Errorf("APOD feed: %w", err)
	}
	defer resp.Body.Close()
	return parseFeed(resp.Body)
}

// fetch streams the picture at u into w.
func (r *refresher) fetch(ctx context.Context, u string, w io.Writer) error {
	resp, err := r.get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

func (r *refresher) get(ctx context.Context, u string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "nightglass")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp, nil
}
