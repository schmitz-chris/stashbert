// Command icongen writes the app icons, the favicon and the iOS startup
// images of the web UI as PNG files.
//
// Usage:
//
//	go run ./tools/icongen -out web/public
//
// The icon shows a pantry shelf on a full-bleed green background: a white
// board with three white items of different heights on it, a tall jar with
// lid and label, a can with two grooves and a wide jar with lid. Details are
// cut out in green. The corners stay square and opaque, iOS rounds them
// itself. The startup images are plain areas in the background color of the
// app, so the launch looks like the first screen (HIG, Launching).
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

var (
	// iconGreen is the accent color of the app (--color-accent in
	// web/src/index.css, emerald-700).
	iconGreen  = color.NRGBA{R: 0x00, G: 0x7a, B: 0x55, A: 0xff} // #007a55
	motifWhite = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// appBackground is the canvas color, the background of html and body. The same
	// value is theme-color in web/index.html and theme_color and
	// background_color in web/vite.config.ts.
	appBackground = color.NRGBA{R: 0xf4, G: 0xf3, B: 0xf1, A: 0xff} // #f4f3f1
)

// Geometry of the motif in thousandths of the side length, x to the right
// and y downwards. The items stand on the shelf with gaps of 40 between
// them; the group spans 200 to 800 horizontally and 250 to 760 vertically.
const (
	unit = 1000 // the side length

	// Shelf board below the items.
	shelfLeft, shelfRight = 150, 850
	shelfTop, shelfBottom = 695, 760
	shelfRadius           = 20

	// Every item stands on the shelf.
	itemBottom = shelfTop

	// Lids of both jars: narrower than the body and separated from it by a
	// green gap.
	lidInset  = 15 // on each side
	lidHeight = 48
	lidGap    = 18
	lidRadius = 14

	// Tall jar on the left, with a label cut out of the body.
	tallLeft, tallRight = 200, 370
	tallTop             = 316 // top of the body
	tallRadius          = 40
	labelInset          = 35 // label to the sides of the body
	labelTop            = 445
	labelBottom         = 575
	labelRadius         = 14

	// Can in the middle, the shortest item, with a groove below its top
	// and above its bottom.
	canLeft, canRight = 410, 570
	canTop            = 465
	canRadius         = 18
	rimHeight         = 34 // from the edge of the can to its groove
	grooveHeight      = 18

	// Wide jar on the right.
	wideLeft, wideRight = 610, 800
	wideTop             = 420 // top of the body
	wideRadius          = 44
)

// samples is the number of samples per pixel along each axis. The share of
// samples inside the motif sets the pixel color, which smooths the edges.
const samples = 8

// icons lists the square icons written to the output directory.
var icons = []struct {
	name string
	size int
}{
	{"apple-touch-icon.png", 180},
	{"icon-192.png", 192},
	{"icon-512.png", 512},
	{"favicon-32.png", 32},
}

// startupImages lists the iOS startup images (apple-touch-startup-image) for
// iPhone 15 (393 x 852 pt) and iPhone 16 Pro (402 x 874 pt), both @3x.
var startupImages = []struct {
	name          string
	width, height int
}{
	{"startup-1179x2556.png", 1179, 2556},
	{"startup-1206x2622.png", 1206, 2622},
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
	for _, s := range startupImages {
		if err := writePNG(filepath.Join(outDir, s.name), renderStartup(s.width, s.height)); err != nil {
			return err
		}
	}
	return nil
}

// roundRect is a rectangle with rounded corners in thousandths of the side
// length. A cut shape is drawn in green over the shapes before it.
type roundRect struct {
	left, top, right, bottom, radius int
	cut                              bool
}

// motif returns the shapes of the icon in drawing order.
func motif() []roundRect {
	shapes := []roundRect{
		{left: shelfLeft, top: shelfTop, right: shelfRight, bottom: shelfBottom, radius: shelfRadius},
	}
	shapes = append(shapes, jar(tallLeft, tallRight, tallTop, tallRadius)...)
	shapes = append(shapes,
		roundRect{left: tallLeft + labelInset, top: labelTop, right: tallRight - labelInset, bottom: labelBottom, radius: labelRadius, cut: true},
		roundRect{left: canLeft, top: canTop, right: canRight, bottom: itemBottom, radius: canRadius},
		roundRect{left: canLeft, top: canTop + rimHeight, right: canRight, bottom: canTop + rimHeight + grooveHeight, cut: true},
		roundRect{left: canLeft, top: itemBottom - rimHeight - grooveHeight, right: canRight, bottom: itemBottom - rimHeight, cut: true},
	)
	return append(shapes, jar(wideLeft, wideRight, wideTop, wideRadius)...)
}

// jar returns the body and the lid of a jar whose body spans left to right
// and top to the shelf.
func jar(left, right, top, radius int) []roundRect {
	return []roundRect{
		{left: left, top: top, right: right, bottom: itemBottom, radius: radius},
		{left: left + lidInset, top: top - lidGap - lidHeight, right: right - lidInset, bottom: top - lidGap, radius: lidRadius},
	}
}

// renderIcon draws the icon with the given side length in pixels.
//
// A sample at index i (of size*samples along an axis) lies at
// (2i+1) / (2*size*samples) of the side, and a motif edge e at e/unit.
// Multiplying both by 2*size*samples*unit keeps every comparison in
// integers, so the pixels are the same on every platform.
func renderIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	shapes := motif()
	scale := int64(2 * size * samples)
	for py := range size {
		for px := range size {
			covered := 0
			for sy := range samples {
				y := int64(2*(py*samples+sy)+1) * unit
				for sx := range samples {
					x := int64(2*(px*samples+sx)+1) * unit
					if inMotif(shapes, x, y, scale) {
						covered++
					}
				}
			}
			img.SetNRGBA(px, py, blend(iconGreen, motifWhite, covered, samples*samples))
		}
	}
	return img
}

// inMotif reports whether the scaled point (x, y) is white.
func inMotif(shapes []roundRect, x, y, scale int64) bool {
	white := false
	for _, s := range shapes {
		if s.contains(x, y, scale) {
			white = !s.cut
		}
	}
	return white
}

// contains reports whether the point (x, y) lies inside r, with the edges of
// r multiplied by scale.
func (r roundRect) contains(x, y, scale int64) bool {
	left, right := int64(r.left)*scale, int64(r.right)*scale
	top, bottom := int64(r.top)*scale, int64(r.bottom)*scale
	if x < left || x >= right || y < top || y >= bottom {
		return false
	}
	// Distance to the inner rectangle that holds the centers of the corner
	// circles; only points in a corner square have one on both axes.
	radius := int64(r.radius) * scale
	dx := max(left+radius-x, x-(right-radius), 0)
	dy := max(top+radius-y, y-(bottom-radius), 0)
	return dx*dx+dy*dy <= radius*radius
}

// blend mixes fg into bg by the share covered/total, rounded per channel.
func blend(bg, fg color.NRGBA, covered, total int) color.NRGBA {
	mix := func(b, f uint8) uint8 {
		return uint8((int(b)*(total-covered) + int(f)*covered + total/2) / total)
	}
	return color.NRGBA{R: mix(bg.R, fg.R), G: mix(bg.G, fg.G), B: mix(bg.B, fg.B), A: 0xff}
}

// renderStartup draws a startup image: the background color of the app
// without logo or text.
func renderStartup(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(appBackground), image.Point{}, draw.Src)
	return img
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
