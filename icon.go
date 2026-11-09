package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The Nightglass logo: a pale crescent moon and a four-point star on a
// deep-indigo rounded square. Bold shapes and high contrast so it still reads
// at 16 px in the tray. docs/logo.svg is the same drawing as a vector.

var (
	logoTop  = color.NRGBA{44, 48, 122, 255}
	logoBot  = color.NRGBA{12, 14, 42, 255}
	logoMoon = color.NRGBA{255, 224, 140, 255}
	logoStar = color.NRGBA{255, 255, 255, 255}
)

// logoAt returns the logo color at (x, y) in unit coordinates, and false outside it.
func logoAt(x, y float64) (color.NRGBA, bool) {
	const r = 0.22 // corner radius
	qx := math.Max(math.Abs(x-0.5)-(0.5-r), 0)
	qy := math.Max(math.Abs(y-0.5)-(0.5-r), 0)
	if math.Hypot(qx, qy) > r {
		return color.NRGBA{}, false
	}
	// Star: the region under sqrt|dx| + sqrt|dy| = sqrt(s), matching the SVG's
	// quadratic curves through the center.
	if dx, dy := math.Abs(x-0.72), math.Abs(y-0.29); math.Sqrt(dx)+math.Sqrt(dy) <= math.Sqrt(0.14) {
		return logoStar, true
	}
	// Moon: a disc with a smaller disc bitten out of its upper right.
	if math.Hypot(x-0.45, y-0.56) <= 0.30 && math.Hypot(x-0.58, y-0.44) > 0.25 {
		return logoMoon, true
	}
	return lerpNRGBA(logoTop, logoBot, y), true
}

func lerpNRGBA(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*t + 0.5) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// drawLogo renders the logo at n×n pixels, anti-aliased by 8×8 supersampling.
func drawLogo(n int) *image.NRGBA {
	const ss = 8
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(n)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(n)
					if c, ok := logoAt(x, y); ok {
						r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
					}
				}
			}
			if a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(b/a + 0.5), uint8(255*a/(ss*ss) + 0.5)})
			}
		}
	}
	return img
}

var logoSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

// makeIcon builds a multi-size .ico (PNG entries) so Windows can pick a sharp
// size for any display scaling.
func makeIcon() []byte {
	var pngs [][]byte
	for _, n := range logoSizes {
		var buf bytes.Buffer
		png.Encode(&buf, drawLogo(n))
		pngs = append(pngs, buf.Bytes())
	}
	var ico bytes.Buffer
	binary.Write(&ico, binary.LittleEndian, []uint16{0, 1, uint16(len(logoSizes))}) // ICONDIR
	offset := 6 + 16*len(logoSizes)
	for i, n := range logoSizes {
		dim := byte(n)                                                                          // 256 is stored as 0
		ico.Write([]byte{dim, dim, 0, 0})                                                       // w, h, colors, reserved
		binary.Write(&ico, binary.LittleEndian, []uint16{1, 32})                                // planes, bpp
		binary.Write(&ico, binary.LittleEndian, []uint32{uint32(len(pngs[i])), uint32(offset)}) // size, offset
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		ico.Write(p)
	}
	return ico.Bytes()
}
