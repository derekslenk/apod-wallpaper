package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry is one APOD day.
type Entry struct {
	Date  string // YYYY-MM-DD
	Title string
	Page  string // article URL on science.nasa.gov
	Image string // full-resolution image URL
}

// The official APOD JSON API broke when APOD moved to science.nasa.gov
// (late 2026); the site's own RSS feed is the reliable source now.
type rssFeed struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		HDURL   string `xml:"https://science.nasa.gov/apod/ hdurl"`
	} `xml:"channel>item"`
}

var slugDate = regexp.MustCompile(`/apod-(\d{4})-([a-z]+)-(\d{1,2})-`)

// parseFeed reads the science.nasa.gov/feed/apod-basic/ RSS feed.
func parseFeed(r io.Reader) ([]Entry, error) {
	var f rssFeed
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("parse APOD feed: %w", err)
	}
	var out []Entry
	for _, it := range f.Items {
		date := ""
		// The article slug carries the APOD date ("apod-2026-november-8-...");
		// pubDate is occasionally a later edit time, so it's only a fallback.
		if m := slugDate.FindStringSubmatch(it.Link); m != nil {
			if t, err := time.Parse("2006-January-2", m[1]+"-"+m[2]+"-"+m[3]); err == nil {
				date = t.Format("2006-01-02")
			}
		}
		if date == "" {
			if t, err := time.Parse(time.RFC1123Z, it.PubDate); err == nil {
				date = t.UTC().Format("2006-01-02")
			}
		}
		if date == "" || it.HDURL == "" {
			continue
		}
		out = append(out, Entry{Date: date, Title: strings.TrimSpace(it.Title), Page: it.Link, Image: fullResURL(it.HDURL)})
	}
	return out, nil
}

// fullResURL turns the feed's resized "dynamicimage" link (capped ~1280px)
// into the original asset: .../dynamicimage/assets/X?w=.. -> .../content/dam/X
func fullResURL(hd string) string {
	u, err := url.Parse(hd)
	if err != nil {
		return hd
	}
	u.RawQuery = ""
	u.Path = strings.Replace(u.Path, "/dynamicimage/assets/", "/content/dam/", 1)
	return u.String()
}

func (e Entry) ext() string {
	if u, err := url.Parse(e.Image); err == nil {
		return strings.ToLower(path.Ext(u.Path))
	}
	return ""
}

func (e Entry) isImage() bool {
	switch e.ext() {
	case ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// localFileName is "YYYY-MM-DD.ext", using the image's own extension.
func (e Entry) localFileName() string {
	if e.isImage() {
		return e.Date + e.ext()
	}
	return e.Date + ".jpg"
}

// pickImages returns up to n image entries, newest first, skipping videos and
// other non-image days.
func pickImages(entries []Entry, n int) []Entry {
	var imgs []Entry
	for _, e := range entries {
		if e.isImage() {
			imgs = append(imgs, e)
		}
	}
	sort.Slice(imgs, func(i, j int) bool { return imgs[i].Date > imgs[j].Date })
	if len(imgs) > n {
		imgs = imgs[:n]
	}
	return imgs
}

// Monitor is a connected display.
type Monitor struct {
	ID            string
	Width, Height int
}

// assignWallpapers maps each monitor to an image path. The largest monitor
// (by pixel area, then width) gets paths[0]; all others get paths[1] when
// available, otherwise paths[0]. Area, not aspect ratio: a short strip display
// like 2560x720 is "wider" than a 3440x1440 ultrawide but isn't the main screen.
func assignWallpapers(monitors []Monitor, paths []string) map[string]string {
	out := map[string]string{}
	if len(monitors) == 0 || len(paths) == 0 {
		return out
	}
	ordered := append([]Monitor(nil), monitors...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ai, aj := ordered[i].Width*ordered[i].Height, ordered[j].Width*ordered[j].Height
		if ai != aj {
			return ai > aj
		}
		return ordered[i].Width > ordered[j].Width
	})
	for i, m := range ordered {
		if i == 0 || len(paths) == 1 {
			out[m.ID] = paths[0]
		} else {
			out[m.ID] = paths[1]
		}
	}
	return out
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
