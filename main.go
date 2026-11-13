//go:build windows

package main

import (
	"context"
	"fmt"
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
	todayPage      = "https://science.nasa.gov/apod/"
)

// version is set at release build time with -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

type app struct {
	r       *refresher
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
	folder := &dataFolder{dir: filepath.Join(base, "Nightglass")}
	migrateErr := folder.migrateFrom(filepath.Join(base, "APOD"))
	if err := os.MkdirAll(folder.dir, 0o755); err != nil {
		os.Exit(1)
	}
	if f, err := os.OpenFile(folder.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
	log.Printf("nightglass %s starting", version)
	if migrateErr != nil {
		log.Printf("move old APOD data folder: %v (starting fresh)", migrateErr)
	}
	if err := ensureAutostart(); err != nil {
		log.Printf("autostart: %v", err)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	a := &app{
		r:       &refresher{folder: folder, feedURL: feedURL, client: client, desktop: setWallpapers},
		client:  client,
		trigger: make(chan struct{}, 1),
	}
	a.style.Store(folder.loadSettings().BarStyle)
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
	if err := a.r.folder.saveSettings(settings{BarStyle: st}); err != nil {
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

// refresh runs one Refresh and reflects it in the tray.
func (a *app) refresh() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	latest, err := a.r.refresh(ctx, a.barStyle())
	if err != nil {
		return err
	}
	tip := "Nightglass: " + latest.Title
	if len(tip) > 120 {
		// The byte cut can split a multi-byte character; drop the partial one.
		tip = strings.ToValidUTF8(tip[:120], "") + "…"
	}
	systray.SetTooltip(tip)
	warnCompetingApps()
	return nil
}
