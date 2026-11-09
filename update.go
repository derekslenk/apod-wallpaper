package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	latestReleaseAPI  = "https://api.github.com/repos/derekslenk/nightglass/releases/latest"
	releasePagePrefix = "https://github.com/derekslenk/nightglass/releases/"
)

// parseVersion reads "vX.Y.Z". Anything else, including "dev", is not a version.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	rest, ok := strings.CutPrefix(s, "v")
	parts := strings.Split(rest, ".")
	if !ok || len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// newerVersion reports whether latest is a strictly newer release than current.
func newerVersion(current, latest string) bool {
	c, ok1 := parseVersion(current)
	l, ok2 := parseVersion(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseLatestRelease reads GitHub's "latest release" response. The page URL is
// opened with ShellExecute, so it must point at this repo's releases on github.com.
func parseLatestRelease(r io.Reader) (tag, page string, err error) {
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(r).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("parse latest release: %w", err)
	}
	if _, ok := parseVersion(rel.TagName); !ok {
		return "", "", fmt.Errorf("latest release tag %q is not vX.Y.Z", rel.TagName)
	}
	if !strings.HasPrefix(rel.HTMLURL, releasePagePrefix) {
		return "", "", fmt.Errorf("unexpected release URL %q", rel.HTMLURL)
	}
	return rel.TagName, rel.HTMLURL, nil
}
