package main

import (
	"image"
	"image/color"
	"testing"
)

func TestFitRectPortraitOnUltrawide(t *testing.T) {
	// 1200x801 image on 3440x1440 -> scaled to 1440 tall, centered horizontally.
	r := fitRect(1200, 801, 3440, 1440)
	if r.Dy() != 1440 || r.Dx() != 2157 {
		t.Fatalf("size %dx%d", r.Dx(), r.Dy())
	}
	if r.Min.X != (3440-2157)/2 || r.Min.Y != 0 {
		t.Fatalf("origin %v", r.Min)
	}
}

func TestFitRectWideImageOnStrip(t *testing.T) {
	// 6000x4076 on 2560x720 -> height-bound: 720 tall.
	r := fitRect(6000, 4076, 2560, 720)
	if r.Dy() != 720 || r.Dx() != 1060 {
		t.Fatalf("size %dx%d", r.Dx(), r.Dy())
	}
	// A panorama wider than the screen gets top/bottom bars.
	r = fitRect(8000, 1000, 2560, 1440)
	if r.Dx() != 2560 || r.Dy() != 320 || r.Min.Y != (1440-320)/2 {
		t.Fatalf("panorama rect %v", r)
	}
}

// vgradient makes a w x h image fading from top color to bottom color.
func vgradient(w, h int, top, bot color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		c := color.RGBA{
			uint8(float64(top.R)*(1-t) + float64(bot.R)*t),
			uint8(float64(top.G)*(1-t) + float64(bot.G)*t),
			uint8(float64(top.B)*(1-t) + float64(bot.B)*t), 255}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

func TestComposeBarsFollowEdgeGradient(t *testing.T) {
	navy, snow := color.RGBA{20, 30, 110, 255}, color.RGBA{220, 225, 240, 255}
	out := compose(vgradient(400, 300, navy, snow), 1600, 600, BarsSoft)
	if out.Bounds() != image.Rect(0, 0, 1600, 600) {
		t.Fatalf("bounds %v", out.Bounds())
	}
	// Both bars: dark at the top, light at the bottom, matching the image edge row by row.
	for _, x := range []int{5, 1595} {
		for _, y := range []int{20, 300, 580} {
			bar, edge := out.RGBAAt(x, y), out.RGBAAt(600, y)
			if !near(bar, edge, 12) {
				t.Errorf("bar(%d,%d)=%v edge=%v", x, y, bar, edge)
			}
		}
	}
	if !near(out.RGBAAt(5, 5), navy, 15) || !near(out.RGBAAt(5, 594), snow, 15) {
		t.Errorf("top %v bottom %v", out.RGBAAt(5, 5), out.RGBAAt(5, 594))
	}
}

func TestComposeBarsIgnoreStars(t *testing.T) {
	navy := color.RGBA{10, 12, 40, 255}
	src := vgradient(400, 300, navy, navy)
	for _, y := range []int{40, 41, 150, 220} { // bright stars right on the edge
		src.SetRGBA(0, y, color.RGBA{255, 255, 255, 255})
		src.SetRGBA(399, y, color.RGBA{255, 255, 255, 255})
	}
	out := compose(src, 1600, 600, BarsSoft)
	for y := 0; y < 600; y++ {
		if c := out.RGBAAt(10, y); !near(c, navy, 6) {
			t.Fatalf("left bar row %d = %v, star leaked", y, c)
		}
		if c := out.RGBAAt(1590, y); !near(c, navy, 6) {
			t.Fatalf("right bar row %d = %v, star leaked", y, c)
		}
	}
}

func TestComposeTopBottomBarsFollowEdge(t *testing.T) {
	// Panorama on a 16:9 screen -> top/bottom bars, following each column.
	src := image.NewRGBA(image.Rect(0, 0, 800, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 800; x++ {
			c := color.RGBA{200, 40, 40, 255}
			if x >= 400 {
				c = color.RGBA{40, 40, 200, 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	out := compose(src, 1600, 900, BarsSoft)
	if c := out.RGBAAt(100, 10); !near(c, color.RGBA{200, 40, 40, 255}, 12) {
		t.Errorf("top-left bar %v", c)
	}
	if c := out.RGBAAt(1500, 890); !near(c, color.RGBA{40, 40, 200, 255}, 12) {
		t.Errorf("bottom-right bar %v", c)
	}
}

func TestComposeBarsBlendHardHorizon(t *testing.T) {
	// Top half dark sky, bottom half bright snow: a hard horizon at mid-height.
	sky, snow := color.RGBA{20, 25, 80, 255}, color.RGBA{230, 230, 240, 255}
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			c := sky
			if y >= 150 {
				c = snow
			}
			src.SetRGBA(x, y, c)
		}
	}
	out := compose(src, 1600, 600, BarsSoft) // horizon lands at row 300
	// Far from the horizon the bar keeps the picture's colors...
	if c := out.RGBAAt(5, 10); !near(c, sky, 20) {
		t.Errorf("top of bar %v, want ~sky", c)
	}
	if c := out.RGBAAt(5, 590); !near(c, snow, 20) {
		t.Errorf("bottom of bar %v, want ~snow", c)
	}
	// ...but 8% of the height away from the horizon it is already clearly
	// blending, rather than holding a crisp band.
	for _, y := range []int{252, 348} {
		c := out.RGBAAt(5, y)
		if near(c, sky, 30) || near(c, snow, 30) {
			t.Errorf("row %d = %v: still a hard band, not a gentle blend", y, c)
		}
	}
}

func TestComposeCrispStyleKeepsHorizonSharp(t *testing.T) {
	sky, snow := color.RGBA{20, 25, 80, 255}, color.RGBA{230, 230, 240, 255}
	src := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			c := sky
			if y >= 150 {
				c = snow
			}
			src.SetRGBA(x, y, c)
		}
	}
	out := compose(src, 1600, 600, BarsCrisp)
	// Crisp: 8% away from the horizon the bar still holds the picture's color.
	if c := out.RGBAAt(5, 252); !near(c, sky, 15) {
		t.Errorf("row 252 = %v, want ~sky", c)
	}
	if c := out.RGBAAt(5, 348); !near(c, snow, 15) {
		t.Errorf("row 348 = %v, want ~snow", c)
	}
}

func TestParseBarStyle(t *testing.T) {
	for in, want := range map[string]BarStyle{"soft": BarsSoft, "crisp": BarsCrisp, "": BarsSoft, "bogus": BarsSoft} {
		if got := parseBarStyle(in); got != want {
			t.Errorf("parseBarStyle(%q) = %q, want %q", in, got, want)
		}
	}
}
