package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// testFeed serves a three-day feed: a video day (newest), then two pictures.
type testFeed struct {
	srv       *httptest.Server
	imageHits atomic.Int32
	failFeed  atomic.Bool
}

func newTestFeed(t *testing.T) *testFeed {
	t.Helper()
	tf := &testFeed{}
	pic := func(w, h int) []byte {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes()
	}
	latest, prev := pic(20, 40), pic(40, 20)
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		if tf.failFeed.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `<rss xmlns:apod="https://science.nasa.gov/apod/"><channel>
<item><title>Video</title><link>https://x/apod-2026-november-9-video/</link><apod:hdurl>%[1]s/video.mp4</apod:hdurl></item>
<item><title>Latest</title><link>https://x/apod-2026-november-8-latest/</link><apod:hdurl>%[1]s/latest.png</apod:hdurl></item>
<item><title>Previous</title><link>https://x/apod-2026-november-7-previous/</link><apod:hdurl>%[1]s/prev.png</apod:hdurl></item>
</channel></rss>`, tf.srv.URL)
	})
	mux.HandleFunc("/latest.png", func(w http.ResponseWriter, r *http.Request) { tf.imageHits.Add(1); w.Write(latest) })
	mux.HandleFunc("/prev.png", func(w http.ResponseWriter, r *http.Request) { tf.imageHits.Add(1); w.Write(prev) })
	tf.srv = httptest.NewServer(mux)
	t.Cleanup(tf.srv.Close)
	return tf
}

// fakeDesktop stands in for setWallpapers: a 64x32 Main monitor and a 32x32
// Side monitor; it records the last files it was asked to apply.
type fakeDesktop struct {
	applied map[string]string
	calls   int
}

func (d *fakeDesktop) set(prepare func([]Monitor) (map[string]string, error)) error {
	d.calls++
	files, err := prepare([]Monitor{{ID: "side", Width: 32, Height: 32}, {ID: "main", Width: 64, Height: 32}})
	if err != nil {
		return err
	}
	d.applied = files
	return nil
}

func newTestRefresher(t *testing.T) (*refresher, *fakeDesktop, *testFeed) {
	t.Helper()
	tf := newTestFeed(t)
	d := &fakeDesktop{}
	return &refresher{folder: &dataFolder{dir: t.TempDir()}, feedURL: tf.srv.URL + "/feed", client: tf.srv.Client(), desktop: d.set}, d, tf
}

func TestRefreshPairsLatestWithMainAndPreviousWithSide(t *testing.T) {
	r, d, _ := newTestRefresher(t)
	latest, err := r.refresh(context.Background(), BarsSoft)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Date != "2026-11-08" {
		t.Errorf("Latest picture = %s, want 2026-11-08 (video day skipped)", latest.Date)
	}
	if !strings.HasPrefix(filepath.Base(d.applied["main"]), "2026-11-08_64x32") {
		t.Errorf("main monitor got %s", d.applied["main"])
	}
	if !strings.HasPrefix(filepath.Base(d.applied["side"]), "2026-11-07_32x32") {
		t.Errorf("side monitor got %s", d.applied["side"])
	}
}

func TestRefreshReusesDownloadsAndRenders(t *testing.T) {
	r, d, tf := newTestRefresher(t)
	if _, err := r.refresh(context.Background(), BarsSoft); err != nil {
		t.Fatal(err)
	}
	first := mtime(t, d.applied["main"])
	if _, err := r.refresh(context.Background(), BarsSoft); err != nil {
		t.Fatal(err)
	}
	if got := tf.imageHits.Load(); got != 2 {
		t.Errorf("image downloads = %d, want 2 (no re-download)", got)
	}
	if mtime(t, d.applied["main"]) != first {
		t.Error("main wallpaper was re-rendered")
	}
}

func TestRefreshPrunesOldDownloadsOnly(t *testing.T) {
	r, _, _ := newTestRefresher(t)
	for i := 1; i <= 8; i++ {
		os.WriteFile(filepath.Join(r.folder.dir, fmt.Sprintf("2026-01-%02d.jpg", i)), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(r.folder.dir, "settings.json"), []byte("{}"), 0o644)
	os.WriteFile(r.folder.logPath(), []byte("log"), 0o644)
	if _, err := r.refresh(context.Background(), BarsSoft); err != nil {
		t.Fatal(err)
	}
	var dated []string
	for _, f := range []string{"settings.json", logName, "2026-11-08.png", "2026-11-07.png"} {
		if _, err := os.Stat(filepath.Join(r.folder.dir, f)); err != nil {
			t.Errorf("%s was deleted", f)
		}
	}
	ents, _ := os.ReadDir(r.folder.dir)
	for _, e := range ents {
		if datedFile.MatchString(e.Name()) {
			dated = append(dated, e.Name())
		}
	}
	if len(dated) != keepImages {
		t.Errorf("dated files after prune = %v, want %d", dated, keepImages)
	}
	if _, err := os.Stat(filepath.Join(r.folder.dir, "2026-01-01.jpg")); err == nil {
		t.Error("oldest download survived the prune")
	}
}

func TestRefreshStyleChangeReplacesRender(t *testing.T) {
	r, d, _ := newTestRefresher(t)
	if _, err := r.refresh(context.Background(), BarsSoft); err != nil {
		t.Fatal(err)
	}
	soft := d.applied["main"]
	if _, err := r.refresh(context.Background(), BarsCrisp); err != nil {
		t.Fatal(err)
	}
	if d.applied["main"] == soft {
		t.Error("style change did not render a new file")
	}
	if _, err := os.Stat(soft); err == nil {
		t.Error("stale soft render was not removed")
	}
}

func TestRefreshFeedErrorTouchesNothing(t *testing.T) {
	r, d, tf := newTestRefresher(t)
	tf.failFeed.Store(true)
	if _, err := r.refresh(context.Background(), BarsSoft); err == nil {
		t.Fatal("want error from a failing feed")
	}
	if d.calls != 0 {
		t.Error("desktop was called despite the feed failing")
	}
}

func mtime(t *testing.T, p string) int64 {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime().UnixNano()
}
