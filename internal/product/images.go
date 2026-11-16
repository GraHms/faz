package product

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxImageSide  = 1600    // pixels on the long side after scaling
	maxInlineSize = 900_000 // bytes; the pipeline inlines assets up to 1 MB
)

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// prepareImage checks an upload, scales large photos down and returns a
// safe file name, the bytes to store and their media type. SVG is refused:
// it can carry scripts, and pages are built from photos and logos anyway.
func prepareImage(filename string, data []byte) (string, []byte, string, error) {
	base := strings.ToLower(filepath.Base(filename))
	ext := filepath.Ext(base)
	stem := strings.Trim(unsafeName.ReplaceAllString(strings.TrimSuffix(base, ext), "-"), "-.")
	if stem == "" {
		stem = "imagem"
	}
	if len(stem) > 60 {
		stem = stem[:60]
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if bytes.HasPrefix(data, []byte("RIFF")) && len(data) > 12 && string(data[8:12]) == "WEBP" {
			if len(data) > maxInlineSize {
				return "", nil, "", errors.New("Envia esta imagem WebP em JPEG ou PNG, ou com menos de 900 KB.")
			}
			return stem + ".webp", data, "image/webp", nil
		}
		return "", nil, "", errors.New("Envia uma imagem JPEG, PNG ou WebP.")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, "", errors.New("Não consegui ler esta imagem.")
	}
	b := img.Bounds()
	small := b.Dx() <= maxImageSide && b.Dy() <= maxImageSide
	if format == "png" && small && len(data) <= maxInlineSize {
		return stem + ".png", data, "image/png", nil
	}
	if format == "jpeg" && small && len(data) <= maxInlineSize {
		return stem + ".jpg", data, "image/jpeg", nil
	}
	scaled := downscale(img, maxImageSide)
	var buf bytes.Buffer
	if format == "png" && hasAlpha(scaled) {
		if err := png.Encode(&buf, scaled); err != nil {
			return "", nil, "", err
		}
		return stem + ".png", buf.Bytes(), "image/png", nil
	}
	for q := 82; q >= 50; q -= 8 {
		buf.Reset()
		if err := jpeg.Encode(&buf, flatten(scaled), &jpeg.Options{Quality: q}); err != nil {
			return "", nil, "", err
		}
		if buf.Len() <= maxInlineSize {
			break
		}
	}
	return stem + ".jpg", buf.Bytes(), "image/jpeg", nil
}

// downscale shrinks img so its long side is at most max, averaging the
// source pixels under each destination pixel (box filter).
func downscale(img image.Image, max int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if w >= h && w > max {
		nw, nh = max, h*max/w
	} else if h > w && h > max {
		nw, nh = w*max/h, max
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 == y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 == x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	return dst
}

func hasAlpha(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return true
		}
	}
	return false
}

// flatten composites transparent pixels onto white for JPEG.
func flatten(img *image.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		a := uint32(img.Pix[i+3])
		for c := 0; c < 3; c++ {
			out.Pix[i+c] = uint8((uint32(img.Pix[i+c])*255 + 255*(255-a)) / 255)
		}
		out.Pix[i+3] = 0xff
	}
	return out
}
