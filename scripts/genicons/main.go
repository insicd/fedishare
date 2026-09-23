// Generates original placeholder icons for FediShare. Safe to re-run.
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	root := "assets"
	if err := os.MkdirAll(root, 0o755); err != nil {
		panic(err)
	}
	mustWritePNG(filepath.Join(root, "icon.png"), render(256, rgb(61, 79, 216)))
	mustWritePNG(filepath.Join(root, "tray-starting.png"), render(32, rgb(142, 142, 147)))
	mustWritePNG(filepath.Join(root, "tray-online.png"), render(32, rgb(52, 199, 89)))
	mustWritePNG(filepath.Join(root, "tray-offline.png"), render(32, rgb(255, 159, 10)))
	mustWritePNG(filepath.Join(root, "tray-error.png"), render(32, rgb(255, 59, 48)))
	mustWritePNG(filepath.Join(root, "tray-paused.png"), render(32, rgb(0, 122, 255)))
	if err := writeICO(filepath.Join(root, "icon.ico"), render(32, rgb(61, 79, 216))); err != nil {
		panic(err)
	}
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 255} }

func render(size int, fill color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)
	inset := size / 10
	radius := size / 5
	roundedRect(img, inset, inset, size-inset, size-inset, radius, fill)

	white := color.RGBA{255, 255, 255, 255}
	cx1, cy := size*35/100, size/2
	cx2 := size * 65 / 100
	dot := size / 9
	fillCircle(img, cx1, cy, dot, white)
	fillCircle(img, cx2, cy, dot, white)
	for x := cx1; x <= cx2; x++ {
		for w := -size / 28; w <= size/28; w++ {
			img.SetRGBA(x, cy+w, white)
		}
	}
	return img
}

func roundedRect(img *image.RGBA, x0, y0, x1, y1, r int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if insideRound(x, y, x0, y0, x1, y1, r) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func insideRound(x, y, x0, y0, x1, y1, r int) bool {
	cx, cy := x, y
	if x < x0+r && y < y0+r {
		return dist2(x, y, x0+r, y0+r) <= r*r
	}
	if x >= x1-r && y < y0+r {
		return dist2(x, y, x1-r-1, y0+r) <= r*r
	}
	if x < x0+r && y >= y1-r {
		return dist2(x, y, x0+r, y1-r-1) <= r*r
	}
	if x >= x1-r && y >= y1-r {
		return dist2(x, y, x1-r-1, y1-r-1) <= r*r
	}
	_ = cx
	_ = cy
	return x >= x0 && x < x1 && y >= y0 && y < y1
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if dist2(x, y, cx, cy) <= r*r {
				if image.Pt(x, y).In(img.Bounds()) {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
}

func dist2(x, y, cx, cy int) int {
	dx, dy := x-cx, y-cy
	return dx*dx + dy*dy
}

func mustWritePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func writeICO(path string, img *image.RGBA) error {
	var pngBuf []byte
	tmp := path + ".png"
	mustWritePNG(tmp, img)
	data, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	_ = os.Remove(tmp)
	pngBuf = data

	b := make([]byte, 6+16+len(pngBuf))
	binary.LittleEndian.PutUint16(b[2:], 1)
	binary.LittleEndian.PutUint16(b[4:], 1)
	b[6] = byte(img.Bounds().Dx())
	b[7] = byte(img.Bounds().Dy())
	b[10] = 1
	b[12] = 32
	binary.LittleEndian.PutUint32(b[14:], uint32(len(pngBuf)))
	binary.LittleEndian.PutUint32(b[18:], 22)
	copy(b[22:], pngBuf)
	return os.WriteFile(path, b, 0o644)
}
