package render

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
)

// Tiles splits a tall screenshot into consecutive JPEG tiles of at most
// tileHeight pixels, keeping at most maxTiles from the top. Models downscale
// very tall images until text is unreadable, so a long page is reviewed as a
// stack of legible tiles instead.
func Tiles(pngBytes []byte, tileHeight, maxTiles, quality int) ([][]byte, int, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, 0, err
	}
	b := img.Bounds()
	total := (b.Dy() + tileHeight - 1) / tileHeight
	var out [][]byte
	for i := 0; i < total && i < maxTiles; i++ {
		y0 := b.Min.Y + i*tileHeight
		y1 := y0 + tileHeight
		if y1 > b.Max.Y {
			y1 = b.Max.Y
		}
		rect := image.Rect(0, 0, b.Dx(), y1-y0)
		dst := image.NewRGBA(rect)
		draw.Draw(dst, rect, img, image.Point{b.Min.X, y0}, draw.Src)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
			return nil, 0, err
		}
		out = append(out, buf.Bytes())
	}
	return out, total, nil
}
