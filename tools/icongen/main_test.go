package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

var green = color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 0xff}

var white = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

func nrgbaAt(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func checkCorners(t *testing.T, img image.Image, size int) {
	t.Helper()
	for _, p := range []image.Point{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
		if got := nrgbaAt(img, p.X, p.Y); got != green {
			t.Errorf("corner %v = %v, want %v", p, got, green)
		}
	}
}

func TestRenderIcon(t *testing.T) {
	for _, size := range []int{180, 192, 512} {
		img := renderIcon(size)
		if got, want := img.Bounds(), image.Rect(0, 0, size, size); got != want {
			t.Fatalf("size %d: bounds = %v, want %v", size, got, want)
		}
		checkCorners(t, img, size)
		// The middle bar covers the center (44 % to 56 %).
		if got := nrgbaAt(img, size/2, size/2); got != white {
			t.Errorf("size %d: center = %v, want %v", size, got, white)
		}
		// The gap between the first and the middle bar spans 27 % to 44 %.
		y := size * 35 / 100
		if got := nrgbaAt(img, size/2, y); got != green {
			t.Errorf("size %d: pixel (%d, %d) between bars = %v, want %v", size, size/2, y, got, green)
		}
	}
}

// TestCheckedInIcons fails when the files in web/public differ from what
// renderIcon draws today. Fix: go run ./tools/icongen -out web/public
func TestCheckedInIcons(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"apple-touch-icon.png", 180},
		{"icon-192.png", 192},
		{"icon-512.png", 512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("..", "..", "web", "public", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := png.Decode(f)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if b, want := got.Bounds(), image.Rect(0, 0, tc.size, tc.size); b != want {
				t.Fatalf("bounds = %v, want %v", b, want)
			}
			checkCorners(t, got, tc.size)
			want := renderIcon(tc.size)
			for y := range tc.size {
				for x := range tc.size {
					if g, w := nrgbaAt(got, x, y), want.NRGBAAt(x, y); g != w {
						t.Fatalf("pixel (%d, %d) = %v, want %v; regenerate with go run ./tools/icongen -out web/public", x, y, g, w)
					}
				}
			}
		})
	}
}
