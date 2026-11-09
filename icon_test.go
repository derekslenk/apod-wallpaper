package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestMakeIconIsValidMultiSizeICO(t *testing.T) {
	ico := makeIcon()
	var hdr [3]uint16
	binary.Read(bytes.NewReader(ico[:6]), binary.LittleEndian, &hdr)
	if hdr[0] != 0 || hdr[1] != 1 || int(hdr[2]) != len(logoSizes) {
		t.Fatalf("ICONDIR = %v", hdr)
	}
	for i, want := range logoSizes {
		e := ico[6+16*i : 6+16*(i+1)]
		size := binary.LittleEndian.Uint32(e[8:12])
		off := binary.LittleEndian.Uint32(e[12:16])
		img, err := png.Decode(bytes.NewReader(ico[off : off+size]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if b := img.Bounds(); b.Dx() != want || b.Dy() != want {
			t.Errorf("entry %d is %v, want %dx%d", i, b, want, want)
		}
		if dim := int(e[0]); dim != want%256 {
			t.Errorf("entry %d width byte = %d, want %d", i, dim, want%256)
		}
	}
}

func TestLogoShapes(t *testing.T) {
	for _, c := range []struct {
		name string
		x, y float64
		want any
	}{
		{"corner is transparent", 0.01, 0.01, nil},
		{"moon body", 0.25, 0.65, logoMoon},
		{"star center", 0.72, 0.29, logoStar},
		{"bite of the moon is sky", 0.55, 0.45, "sky"},
	} {
		col, ok := logoAt(c.x, c.y)
		switch w := c.want.(type) {
		case nil:
			if ok {
				t.Errorf("%s: got %v", c.name, col)
			}
		case string:
			if !ok || col == logoMoon || col == logoStar {
				t.Errorf("%s: got %v %v", c.name, col, ok)
			}
		default:
			if !ok || col != w {
				t.Errorf("%s: got %v %v", c.name, col, ok)
			}
		}
	}
}
