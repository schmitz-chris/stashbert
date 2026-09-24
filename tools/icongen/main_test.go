package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

var (
	green   = color.NRGBA{R: 0x00, G: 0x7a, B: 0x55, A: 0xff} // #007a55
	white   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	stone50 = color.NRGBA{R: 0xfa, G: 0xfa, B: 0xf9, A: 0xff} // #fafaf9
)

// wantIcons and wantStartupImages list the files in web/public that icongen
// writes, with their sizes in pixels.
var wantIcons = []struct {
	name string
	size int
}{
	{"apple-touch-icon.png", 180},
	{"icon-192.png", 192},
	{"icon-512.png", 512},
	{"favicon-32.png", 32},
}

var wantStartupImages = []struct {
	name          string
	width, height int
}{
	{"startup-1179x2556.png", 1179, 2556}, // iPhone 15, 393 x 852 pt @3x
	{"startup-1206x2622.png", 1206, 2622}, // iPhone 16 Pro, 402 x 874 pt @3x
}

// motifPixels are points of the motif in thousandths of the side length,
// each in the middle of an area that is wide enough to give a pure color
// from 180 px up.
var motifPixels = []struct {
	name string
	x, y int
	want color.NRGBA
}{
	{"shelf", (shelfLeft + shelfRight) / 2, (shelfTop + shelfBottom) / 2, white},
	{"tall jar below the label", (tallLeft + tallRight) / 2, (labelBottom + itemBottom) / 2, white},
	{"tall jar label", (tallLeft + tallRight) / 2, (labelTop + labelBottom) / 2, green},
	{"tall jar lid", (tallLeft + tallRight) / 2, tallTop - lidGap - lidHeight/2, white},
	{"gap between lid and tall jar", (tallLeft + tallRight) / 2, tallTop - lidGap/2, green},
	{"gap between tall jar and can", (tallRight + canLeft) / 2, (canTop + itemBottom) / 2, green},
	{"can", (canLeft + canRight) / 2, (canTop + itemBottom) / 2, white},
	{"above the can", (canLeft + canRight) / 2, canTop / 2, green},
	{"wide jar", (wideLeft + wideRight) / 2, (wideTop + itemBottom) / 2, white},
	{"gap between can and wide jar", (canRight + wideLeft) / 2, (wideTop + itemBottom) / 2, green},
	{"below the shelf", (shelfLeft + shelfRight) / 2, (shelfBottom + unit) / 2, green},
}

func nrgbaAt(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

// checkIcon checks the size, the corners and the opacity of an icon.
func checkIcon(t *testing.T, img image.Image, size int) {
	t.Helper()
	if got, want := img.Bounds(), image.Rect(0, 0, size, size); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	for _, p := range []image.Point{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
		if got := nrgbaAt(img, p.X, p.Y); got != green {
			t.Errorf("corner %v = %v, want %v", p, got, green)
		}
	}
	for y := range size {
		for x := range size {
			if a := nrgbaAt(img, x, y).A; a != 0xff {
				t.Fatalf("pixel (%d, %d) has alpha %d, want an opaque icon", x, y, a)
			}
		}
	}
}

// checkSame fails at the first pixel where got differs from want.
func checkSame(t *testing.T, got image.Image, want *image.NRGBA) {
	t.Helper()
	b := want.Bounds()
	if got.Bounds() != b {
		t.Fatalf("bounds = %v, want %v", got.Bounds(), b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if g, w := nrgbaAt(got, x, y), want.NRGBAAt(x, y); g != w {
				t.Fatalf("pixel (%d, %d) = %v, want %v; regenerate with go run ./tools/icongen -out web/public", x, y, g, w)
			}
		}
	}
}

func readPublic(t *testing.T, name string) image.Image {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "web", "public", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func TestRenderIcon(t *testing.T) {
	for _, icon := range wantIcons {
		img := renderIcon(icon.size)
		checkIcon(t, img, icon.size)
		if icon.size < 180 {
			continue // the favicon is too small for pure motif pixels
		}
		for _, p := range motifPixels {
			x, y := p.x*icon.size/unit, p.y*icon.size/unit
			if got := img.NRGBAAt(x, y); got != p.want {
				t.Errorf("size %d: %s at (%d, %d) = %v, want %v", icon.size, p.name, x, y, got, p.want)
			}
		}
	}
}

func TestRenderStartup(t *testing.T) {
	for _, s := range wantStartupImages {
		img := renderStartup(s.width, s.height)
		if got, want := img.Bounds(), image.Rect(0, 0, s.width, s.height); got != want {
			t.Fatalf("%s: bounds = %v, want %v", s.name, got, want)
		}
		for y := range s.height {
			for x := range s.width {
				if got := img.NRGBAAt(x, y); got != stone50 {
					t.Fatalf("%s: pixel (%d, %d) = %v, want %v", s.name, x, y, got, stone50)
				}
			}
		}
	}
}

// TestCheckedInFiles fails when a file in web/public differs from what
// icongen draws today. Fix: go run ./tools/icongen -out web/public
func TestCheckedInFiles(t *testing.T) {
	for _, icon := range wantIcons {
		t.Run(icon.name, func(t *testing.T) {
			got := readPublic(t, icon.name)
			checkIcon(t, got, icon.size)
			checkSame(t, got, renderIcon(icon.size))
		})
	}
	for _, s := range wantStartupImages {
		t.Run(s.name, func(t *testing.T) {
			checkSame(t, readPublic(t, s.name), renderStartup(s.width, s.height))
		})
	}
}
