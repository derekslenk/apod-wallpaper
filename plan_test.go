package main

import (
	"reflect"
	"strings"
	"testing"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:apod="https://science.nasa.gov/apod/">
<channel>
	<title>APOD Basic</title>
	<item>
		<title>Major Fireball Meteor</title>
		<link>https://science.nasa.gov/image-article/apod-2026-november-8-major-fireball-meteor/</link>
		<pubDate>Sun, 08 Nov 2026 05:05:00 +0000</pubDate>
		<apod:hdurl>https://assets.science.nasa.gov/dynamicimage/assets/science/cds/apod/apod/2026/november/meteor2017_Taylor_1200.jpg?w=0&#038;h=0&#038;fit=clip</apod:hdurl>
	</item>
	<item>
		<title>Dark Matter in a Simulated Universe</title>
		<link>https://science.nasa.gov/image-article/apod-2026-october-25-dark-matter/</link>
		<pubDate>Sun, 25 Oct 2026 08:19:48 +0000</pubDate>
		<apod:hdurl>https://assets.science.nasa.gov/content/dam/science/cds/apod/apod/2021/october/DarkMatter_KipacAmnh_1200.jpg?w=1200</apod:hdurl>
	</item>
	<item>
		<title>No Slug Date</title>
		<link>https://science.nasa.gov/image-article/something-else/</link>
		<pubDate>Sat, 24 Oct 2026 04:05:00 +0000</pubDate>
		<apod:hdurl>https://assets.science.nasa.gov/content/dam/x/clip.mp4</apod:hdurl>
	</item>
</channel>
</rss>`

func TestParseFeed(t *testing.T) {
	got, err := parseFeed(strings.NewReader(sampleFeed))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Date: "2026-11-08", Title: "Major Fireball Meteor",
			Page:  "https://science.nasa.gov/image-article/apod-2026-november-8-major-fireball-meteor/",
			Image: "https://assets.science.nasa.gov/content/dam/science/cds/apod/apod/2026/november/meteor2017_Taylor_1200.jpg"},
		{Date: "2026-10-25", Title: "Dark Matter in a Simulated Universe",
			Page:  "https://science.nasa.gov/image-article/apod-2026-october-25-dark-matter/",
			Image: "https://assets.science.nasa.gov/content/dam/science/cds/apod/apod/2021/october/DarkMatter_KipacAmnh_1200.jpg"},
		{Date: "2026-10-24", Title: "No Slug Date",
			Page:  "https://science.nasa.gov/image-article/something-else/",
			Image: "https://assets.science.nasa.gov/content/dam/x/clip.mp4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestPickImagesSkipsVideosNewestFirst(t *testing.T) {
	entries := []Entry{
		{Date: "2026-11-06", Image: "https://x/a.jpg"},
		{Date: "2026-11-08", Image: "https://x/clip.mp4"},
		{Date: "2026-11-07", Image: "https://x/b.PNG"},
		{Date: "2026-11-05", Image: "https://x/c.jpg"},
	}
	got := pickImages(entries, 2)
	if len(got) != 2 || got[0].Date != "2026-11-07" || got[1].Date != "2026-11-06" {
		t.Fatalf("got %+v", got)
	}
}

func TestLocalFileName(t *testing.T) {
	cases := map[string]string{
		"https://x/apod/2026/november/M31.JPG": "2026-11-08.jpg",
		"https://x/apod/2026/november/x.png":   "2026-11-08.png",
		"https://x/apod/2026/november/noext":   "2026-11-08.jpg",
	}
	for u, want := range cases {
		e := Entry{Date: "2026-11-08", Image: u}
		if got := e.localFileName(); got != want {
			t.Errorf("%s: got %s want %s", u, got, want)
		}
	}
}

func TestAssignWallpapersUltrawideGetsToday(t *testing.T) {
	mons := []Monitor{
		{ID: "side", Width: 2560, Height: 1440},
		{ID: "ultra", Width: 3440, Height: 1440},
	}
	got := assignWallpapers(mons, []string{"today", "yesterday"})
	want := map[string]string{"ultra": "today", "side": "yesterday"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAssignWallpapersUltrawideBeatsStripDisplay(t *testing.T) {
	// The real setup: 2560x1440, 3440x1440 ultrawide, 2560x720 strip.
	mons := []Monitor{
		{ID: "side", Width: 2560, Height: 1440},
		{ID: "ultra", Width: 3440, Height: 1440},
		{ID: "strip", Width: 2560, Height: 720},
	}
	got := assignWallpapers(mons, []string{"today", "yesterday"})
	want := map[string]string{"ultra": "today", "side": "yesterday", "strip": "yesterday"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAssignWallpapersSingleImage(t *testing.T) {
	mons := []Monitor{{ID: "a", Width: 1920, Height: 1080}, {ID: "b", Width: 1920, Height: 1080}}
	got := assignWallpapers(mons, []string{"today"})
	if got["a"] != "today" || got["b"] != "today" {
		t.Fatalf("got %v", got)
	}
}

func TestFilesToDelete(t *testing.T) {
	names := []string{
		"2026-11-08.jpg", "2026-11-07.jpg", "2026-11-06.png", "2026-11-05.jpg",
		"config.json", "nightglass.log", "2026-11-01.jpg",
	}
	got := filesToDelete(names, 3, map[string]bool{"2026-11-01.jpg": true})
	want := []string{"2026-11-05.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
