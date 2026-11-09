//go:build windows

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
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

const (
	checkInterval  = 3 * time.Hour
	retryInterval  = 5 * time.Minute
	updateInterval = 24 * time.Hour
	keepImages     = 7
	todayPage      = "https://science.nasa.gov/apod/"
	feedURL        = "https://science.nasa.gov/feed/apod-basic/"
)

// version is set at release build time with -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

type app struct {
	dir     string
	client  *http.Client
	trigger chan struct{}
	mu      sync.Mutex   // serializes refreshes
	style   atomic.Value // BarStyle; set from the tray menu, read by refresh
	update  atomic.Value // string: page of a newer release, once one is found
}

func main() {
	// Checked before touching the data folder, so a running copy never has it open
	// while it is migrated.
	if !singleInstance() {
		return
	}
	base := os.Getenv("LOCALAPPDATA")
	dir := filepath.Join(base, "Nightglass")
	migrateErr := migrateDataDir(filepath.Join(base, "APOD"), dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		os.Exit(1)
	}
	if f, err := os.OpenFile(filepath.Join(dir, logName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
	log.Printf("nightglass %s starting", version)
	if migrateErr != nil {
		log.Printf("move old APOD data folder: %v (starting fresh)", migrateErr)
	}
	if err := ensureAutostart(); err != nil {
		log.Printf("autostart: %v", err)
	}
	a := &app{
		dir:     dir,
		client:  &http.Client{Timeout: 2 * time.Minute},
		trigger: make(chan struct{}, 1),
	}
	a.style.Store(loadSettings(dir).BarStyle)
	systray.Run(a.onReady, func() {})
}

func (a *app) onReady() {
	systray.SetIcon(makeIcon())
	systray.SetTooltip("Nightglass")
	systray.AddMenuItem("Nightglass "+version, "").Disable()
	mUpdate := systray.AddMenuItem("Update available", "Open the release page")
	mUpdate.Hide()
	systray.AddSeparator()
	mRefresh := systray.AddMenuItem("Refresh now", "Fetch the latest APOD")
	mOpen := systray.AddMenuItem("Open today's APOD page", "Open science.nasa.gov/apod")
	mStyle := systray.AddMenuItem("Bar style", "How the side bars blend with the picture")
	mSoft := mStyle.AddSubMenuItemCheckbox("Soft blend", "Bars fade gently, horizons blur out", false)
	mCrisp := mStyle.AddSubMenuItemCheckbox("Crisp follow", "Bars closely track the picture's edge", false)
	showStyle := func(st BarStyle) {
		if st == BarsCrisp {
			mCrisp.Check()
			mSoft.Uncheck()
		} else {
			mSoft.Check()
			mCrisp.Uncheck()
		}
	}
	showStyle(a.barStyle())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit Nightglass")

	go func() {
		for {
			select {
			case <-mUpdate.ClickedCh:
				if u, _ := a.update.Load().(string); u != "" {
					openURL(u)
				}
			case <-mRefresh.ClickedCh:
				a.requestRefresh()
			case <-mOpen.ClickedCh:
				openURL(todayPage)
			case <-mSoft.ClickedCh:
				a.setBarStyle(BarsSoft)
				showStyle(BarsSoft)
			case <-mCrisp.ClickedCh:
				a.setBarStyle(BarsCrisp)
				showStyle(BarsCrisp)
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
	go a.loop()
	go a.updateLoop(mUpdate)
}

// updateLoop checks GitHub for a newer release at startup and then daily, and
// shows mUpdate when one exists. Failures only log; the next check is a day later.
func (a *app) updateLoop(mUpdate *systray.MenuItem) {
	if _, ok := parseVersion(version); !ok {
		log.Printf("update check skipped: version %q is not a release build", version)
		return
	}
	for {
		if tag, page, err := a.latestRelease(); err != nil {
			log.Printf("update check failed: %v", err)
		} else if newerVersion(version, tag) {
			a.update.Store(page)
			mUpdate.SetTitle("Update available: " + tag)
			mUpdate.Show()
			log.Printf("update available: %s", tag)
		}
		time.Sleep(updateInterval)
	}
}

func (a *app) latestRelease() (tag, page string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", latestReleaseAPI, nil)
	req.Header.Set("User-Agent", "nightglass")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub releases: HTTP %s", resp.Status)
	}
	return parseLatestRelease(resp.Body)
}

func (a *app) barStyle() BarStyle { return a.style.Load().(BarStyle) }

// setBarStyle saves the choice and re-renders the wallpapers with it.
func (a *app) setBarStyle(st BarStyle) {
	if st == a.barStyle() {
		return
	}
	a.style.Store(st)
	if err := saveSettings(a.dir, settings{BarStyle: st}); err != nil {
		log.Printf("save settings: %v", err)
	}
	log.Printf("bar style -> %s", st)
	a.requestRefresh()
}

func (a *app) requestRefresh() {
	select {
	case a.trigger <- struct{}{}:
	default:
	}
}

func (a *app) loop() {
	timer := time.NewTimer(0)
	for {
		select {
		case <-timer.C:
		case <-a.trigger:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		next := checkInterval
		if err := a.refresh(); err != nil {
			log.Printf("refresh failed: %v", err)
			next = retryInterval
		}
		timer.Reset(next)
	}
}

func (a *app) refresh() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	entries, err := a.fetchEntries(ctx)
	if err != nil {
		return err
	}
	imgs := pickImages(entries, 2)
	if len(imgs) == 0 {
		return fmt.Errorf("no image entries in the APOD feed")
	}

	var paths []string
	inUse := map[string]bool{}
	for _, e := range imgs {
		p := filepath.Join(a.dir, e.localFileName())
		if _, err := os.Stat(p); err != nil {
			if err := a.download(ctx, e.Image, p); err != nil {
				return fmt.Errorf("download %s: %w", e.Date, err)
			}
			log.Printf("downloaded %s %q", e.Date, e.Title)
		}
		paths = append(paths, p)
		inUse[filepath.Base(p)] = true
	}

	if err := setWallpapers(func(mons []Monitor) (map[string]string, error) {
		return a.renderForMonitors(mons, paths)
	}); err != nil {
		return err
	}
	tip := "Nightglass: " + imgs[0].Title
	if len(tip) > 120 {
		// The byte cut can split a multi-byte character; drop the partial one.
		tip = strings.ToValidUTF8(tip[:120], "") + "…"
	}
	systray.SetTooltip(tip)
	log.Printf("wallpaper set: today=%s", imgs[0].Date)
	warnCompetingApps()

	if names, err := os.ReadDir(a.dir); err == nil {
		var ns []string
		for _, n := range names {
			ns = append(ns, n.Name())
		}
		for _, n := range filesToDelete(ns, keepImages, inUse) {
			os.Remove(filepath.Join(a.dir, n))
		}
	}
	return nil
}

// renderForMonitors builds a centered, bar-matched wallpaper for each monitor
// at its exact resolution, in <dir>\rendered\. Renders are cached by
// source+size; stale ones are removed.
func (a *app) renderForMonitors(mons []Monitor, paths []string) (map[string]string, error) {
	rdir := filepath.Join(a.dir, "rendered")
	if err := os.MkdirAll(rdir, 0o755); err != nil {
		return nil, err
	}
	style := a.barStyle()
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

func (a *app) fetchEntries(ctx context.Context) ([]Entry, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	req.Header.Set("User-Agent", "nightglass")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("APOD feed: HTTP %s", resp.Status)
	}
	return parseFeed(resp.Body)
}

func (a *app) download(ctx context.Context, u, dest string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "nightglass")
	resp, err := a.client.Do(req)
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
