// Command icongen writes the app icons of the web UI as PNG files.
//
// Usage:
//
//	go run ./tools/icongen -out web/public
//
// The motif is a full-bleed green background with three white horizontal
// bars as a shelf. The corners stay square and opaque, iOS rounds them itself.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// Measurements in percent of the side length. The bars form one group that
// keeps marginPercent to all four edges. The inner area (70 %) holds the
// bars at its top and bottom edge and one in the middle, with two equal gaps.
const (
	marginPercent = 15 // empty border around the bar group on every side
	barPercent    = 12 // height of one bar
	barCount      = 3
	// gapPercent is the space between two bars: (100 - 2*15 - 3*12) / 2 = 17.
	gapPercent = (100 - 2*marginPercent - barCount*barPercent) / (barCount - 1)
)

var (
	background = color.NRGBA{R: 0x15, G: 0x80, B: 0x3d, A: 0xff} // #15803d
	barColor   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// icons lists the files written to the output directory.
var icons = []struct {
	name string
	size int
}{
	{"apple-touch-icon.png", 180},
	{"icon-192.png", 192},
	{"icon-512.png", 512},
}

func main() {
	out := flag.String("out", "", "output directory for the PNG files")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
}

func run(outDir string) error {
	if outDir == "" {
		return errors.New("flag -out is required")
	}
	for _, icon := range icons {
		if err := writePNG(filepath.Join(outDir, icon.name), renderIcon(icon.size)); err != nil {
			return err
		}
	}
	return nil
}

// renderIcon draws the icon with the given side length in pixels.
func renderIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.SetNRGBA(x, y, background)
		}
	}
	left := px(marginPercent, size)
	right := px(100-marginPercent, size)
	for i := range barCount {
		topPercent := marginPercent + i*(barPercent+gapPercent)
		top := px(topPercent, size)
		bottom := px(topPercent+barPercent, size)
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				img.SetNRGBA(x, y, barColor)
			}
		}
	}
	return img
}

// px converts a percentage of size to a pixel edge, rounded to the nearest
// pixel.
func px(percent, size int) int {
	return (percent*size + 50) / 100
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
