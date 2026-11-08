package main

import (
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.1.0", true},
		{"v1.9.0", "v1.10.0", true},
		{"v1.2.3", "v2.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.1.0", "v1.0.9", false},
		{"v2.0.0", "v1.9.9", false},
		{"dev", "v9.9.9", false},
		{"v1.0.0", "dev", false},
		{"1.0.0", "v2.0.0", false},
		{"v1.0", "v2.0.0", false},
		{"v1.0.0", "v2.0.0-rc.1", false},
		{"v1.0.0", "v+2.0.0", false},
		{"", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.current, c.latest); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestParseLatestRelease(t *testing.T) {
	tag, page, err := parseLatestRelease(strings.NewReader(
		`{"tag_name":"v1.2.0","html_url":"https://github.com/derekslenk/apod-wallpaper/releases/tag/v1.2.0","assets":[]}`))
	if err != nil || tag != "v1.2.0" || page != "https://github.com/derekslenk/apod-wallpaper/releases/tag/v1.2.0" {
		t.Fatalf("got %q %q %v", tag, page, err)
	}
	for _, body := range []string{
		`not json`,
		`{"tag_name":"latest","html_url":"https://github.com/derekslenk/apod-wallpaper/releases/tag/latest"}`,
		`{"tag_name":"v1.2.0","html_url":"https://evil.example/releases/tag/v1.2.0"}`,
		`{"tag_name":"v1.2.0","html_url":"C:\\Windows\\System32\\calc.exe"}`,
	} {
		if _, _, err := parseLatestRelease(strings.NewReader(body)); err == nil {
			t.Errorf("parseLatestRelease(%s) accepted bad input", body)
		}
	}
}
