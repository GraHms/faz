package check

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RGB is an sRGB color with channels from 0 to 255.
type RGB struct{ R, G, B float64 }

// ParseHex reads #RRGGBB or #RGB.
func ParseHex(s string) (RGB, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return RGB{}, fmt.Errorf("color %q is not #RRGGBB", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return RGB{}, fmt.Errorf("color %q is not hex: %w", s, err)
	}
	return RGB{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}, nil
}

// Hex formats the color as #RRGGBB.
func (c RGB) Hex() string {
	clamp := func(x float64) int { return int(math.Round(math.Max(0, math.Min(255, x)))) }
	return fmt.Sprintf("#%02X%02X%02X", clamp(c.R), clamp(c.G), clamp(c.B))
}

// Luminance is WCAG 2 relative luminance.
func (c RGB) Luminance() float64 {
	ch := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// Contrast is the WCAG 2 contrast ratio between two colors, from 1 to 21.
func Contrast(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Mix moves a toward b by t, from 0 (a) to 1 (b).
func Mix(a, b RGB, t float64) RGB {
	return RGB{a.R + (b.R-a.R)*t, a.G + (b.G-a.G)*t, a.B + (b.B-a.B)*t}
}

// Distance is a perceptually weighted RGB distance ("redmean"). Around 30
// reads as clearly similar; under 15 as nearly identical.
func Distance(a, b RGB) float64 {
	rm := (a.R + b.R) / 2
	dr, dg, db := a.R-b.R, a.G-b.G, a.B-b.B
	return math.Sqrt((2+rm/256)*dr*dr+4*dg*dg+(2+(255-rm)/256)*db*db) / 3
}

// HSL returns hue in degrees, saturation and lightness from 0 to 1.
func (c RGB) HSL() (h, s, l float64) {
	r, g, b := c.R/255, c.G/255, c.B/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

// FixContrast nudges fg toward black or white, whichever direction raises the
// ratio against bg, until it reaches target. It returns the adjusted color
// and whether the target was reached.
func FixContrast(fg, bg RGB, target float64) (RGB, bool) {
	if Contrast(fg, bg) >= target {
		return fg, true
	}
	black, white := RGB{0, 0, 0}, RGB{255, 255, 255}
	toward := white
	if Contrast(black, bg) > Contrast(white, bg) {
		toward = black
	}
	for t := 0.04; t <= 1.0001; t += 0.04 {
		c := Mix(fg, toward, t)
		if Contrast(c, bg) >= target {
			return c, true
		}
	}
	return toward, Contrast(toward, bg) >= target
}
