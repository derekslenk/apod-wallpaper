package main

import (
	"image"
	"image/color"
	"math"
	"slices"

	"golang.org/x/image/draw"
)

// fitRect scales an imgW x imgH picture to fit entirely inside monW x monH
// (no cropping) and centers it.
func fitRect(imgW, imgH, monW, monH int) image.Rectangle {
	s := math.Min(float64(monW)/float64(imgW), float64(monH)/float64(imgH))
	w := min(int(math.Round(float64(imgW)*s)), monW)
	h := min(int(math.Round(float64(imgH)*s)), monH)
	x, y := (monW-w)/2, (monH-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// BarStyle picks how the bars follow the picture's edge.
type BarStyle string

const (
	BarsSoft  BarStyle = "soft"  // wide blend: sharp features fade gently
	BarsCrisp BarStyle = "crisp" // tight follow: bars track the edge closely
)

func parseBarStyle(s string) BarStyle {
	if BarStyle(s) == BarsCrisp {
		return BarsCrisp
	}
	return BarsSoft
}

// blurDivisor sets the smoothing radius as a fraction of the edge length.
func (b BarStyle) blurDivisor() int {
	if b == BarsCrisp {
		return 60
	}
	return 12
}

// compose renders src centered on a monW x monH canvas. The bars are painted
// line by line with the color of the picture's adjacent edge, smoothed along
// the edge, so they fade with the picture (e.g. sky at top, snow at bottom).
func compose(src image.Image, monW, monH int, style BarStyle) *image.RGBA {
	sb := src.Bounds()
	dst := fitRect(sb.Dx(), sb.Dy(), monW, monH)
	out := image.NewRGBA(image.Rect(0, 0, monW, monH))
	draw.CatmullRom.Scale(out, dst, src, sb, draw.Src, nil)

	if dst.Dx() < monW { // side bars
		left := edgeProfile(out, dst, true, false, style.blurDivisor())
		right := edgeProfile(out, dst, true, true, style.blurDivisor())
		for i, y := 0, dst.Min.Y; y < dst.Max.Y; i, y = i+1, y+1 {
			fillRow(out, 0, dst.Min.X, y, left[i])
			fillRow(out, dst.Max.X, monW, y, right[i])
		}
	}
	if dst.Dy() < monH { // top/bottom bars
		top := edgeProfile(out, dst, false, false, style.blurDivisor())
		bot := edgeProfile(out, dst, false, true, style.blurDivisor())
		for i, x := 0, dst.Min.X; x < dst.Max.X; i, x = i+1, x+1 {
			fillCol(out, x, 0, dst.Min.Y, top[i])
			fillCol(out, x, dst.Max.Y, monH, bot[i])
		}
	}
	return out
}

// edgeProfile returns one color per line along an edge of the placed picture:
// per row for a left/right edge (vertical=true), per column for top/bottom.
// Each line's color is the per-channel median of a thin strip at the edge
// (so stars don't count), then the profile is smoothed along the edge.
func edgeProfile(img *image.RGBA, r image.Rectangle, vertical, far bool, blurDiv int) []color.RGBA {
	var n, depth int
	if vertical {
		n, depth = r.Dy(), max(r.Dx()/60, 3)
	} else {
		n, depth = r.Dx(), max(r.Dy()/60, 3)
	}
	at := func(i, d int) color.RGBA {
		if vertical {
			x := r.Min.X + d
			if far {
				x = r.Max.X - 1 - d
			}
			return img.RGBAAt(x, r.Min.Y+i)
		}
		y := r.Min.Y + d
		if far {
			y = r.Max.Y - 1 - d
		}
		return img.RGBAAt(r.Min.X+i, y)
	}

	ch := [3][]float64{make([]float64, n), make([]float64, n), make([]float64, n)}
	rs, gs, bs := make([]uint8, depth), make([]uint8, depth), make([]uint8, depth)
	for i := 0; i < n; i++ {
		for d := 0; d < depth; d++ {
			c := at(i, d)
			rs[d], gs[d], bs[d] = c.R, c.G, c.B
		}
		ch[0][i], ch[1][i], ch[2][i] = median(rs), median(gs), median(bs)
	}
	// Median along the edge first (kills short runs like stars or a meteor
	// trail crossing the edge), then three box blurs (~Gaussian with sigma
	// ~edge/blurDiv): soft (/12) turns a horizon into a gentle blend, crisp
	// (/60) keeps it as a sharper band.
	win := max(n/100, 2)
	for c := range ch {
		ch[c] = medianFilter(ch[c], win)
		for k := 0; k < 3; k++ {
			ch[c] = boxBlur(ch[c], max(n/blurDiv, 1))
		}
	}
	out := make([]color.RGBA, n)
	for i := range out {
		out[i] = color.RGBA{clamp8(ch[0][i]), clamp8(ch[1][i]), clamp8(ch[2][i]), 255}
	}
	return out
}

func median(v []uint8) float64 {
	s := append([]uint8(nil), v...)
	slices.Sort(s)
	return float64(s[len(s)/2])
}

func medianFilter(v []float64, rad int) []float64 {
	out := make([]float64, len(v))
	buf := make([]float64, 0, 2*rad+1)
	for i := range v {
		buf = buf[:0]
		for j := max(i-rad, 0); j <= min(i+rad, len(v)-1); j++ {
			buf = append(buf, v[j])
		}
		slices.Sort(buf)
		out[i] = buf[len(buf)/2]
	}
	return out
}

func boxBlur(v []float64, rad int) []float64 {
	n := len(v)
	prefix := make([]float64, n+1)
	for i, x := range v {
		prefix[i+1] = prefix[i] + x
	}
	out := make([]float64, n)
	for i := range v {
		// Shrink the window symmetrically near the ends so the result isn't
		// pulled toward the middle (keeps the corners true to the picture).
		r := min(rad, i, n-1-i)
		lo, hi := i-r, i+r
		out[i] = (prefix[hi+1] - prefix[lo]) / float64(hi-lo+1)
	}
	return out
}

func clamp8(f float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(f)))) }

func fillRow(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	for x := x0; x < x1; x++ {
		img.SetRGBA(x, y, c)
	}
}

func fillCol(img *image.RGBA, x, y0, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		img.SetRGBA(x, y, c)
	}
}
