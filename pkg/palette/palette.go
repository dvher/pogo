// Package palette lists the note colors shared by every Pogo app. Besides
// the named colors, a note can have any "#rrggbb" background; its text and
// accent colors are then derived from it. Keep in sync with
// frontend/src/lib/colors.ts.
package palette

import (
	"math"
	"regexp"
	"strconv"
)

// Color is a note color: background, text and accent as RGB.
type Color struct {
	Name   string
	BG     [3]uint8
	Ink    [3]uint8
	Accent [3]uint8
}

// Default is the color of new notes and of notes with an unknown color.
const Default = "yellow"

// Colors in menu order.
var Colors = []Color{
	{"yellow", [3]uint8{0xff, 0xf1, 0x76}, [3]uint8{0x3e, 0x35, 0x00}, [3]uint8{0xc7, 0xb2, 0x00}},
	{"pink", [3]uint8{0xf8, 0xbb, 0xd0}, [3]uint8{0x4a, 0x10, 0x27}, [3]uint8{0xd0, 0x66, 0x8c}},
	{"green", [3]uint8{0xc5, 0xe1, 0xa5}, [3]uint8{0x1f, 0x3a, 0x0b}, [3]uint8{0x6f, 0x9e, 0x3f}},
	{"blue", [3]uint8{0xb3, 0xe5, 0xfc}, [3]uint8{0x0b, 0x33, 0x46}, [3]uint8{0x4a, 0x9c, 0xc2}},
	{"purple", [3]uint8{0xd1, 0xc4, 0xe9}, [3]uint8{0x2b, 0x1a, 0x4d}, [3]uint8{0x7e, 0x67, 0xb3}},
	{"orange", [3]uint8{0xff, 0xcc, 0x80}, [3]uint8{0x4a, 0x28, 0x00}, [3]uint8{0xd0, 0x8a, 0x24}},
	{"gray", [3]uint8{0xe0, 0xe0, 0xe0}, [3]uint8{0x26, 0x26, 0x26}, [3]uint8{0x8a, 0x8a, 0x8a}},
}

var custom = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Valid reports whether name is a known color or a lowercase "#rrggbb".
func Valid(name string) bool {
	if custom.MatchString(name) {
		return true
	}
	for _, c := range Colors {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Get returns the named or custom color, or the default one.
func Get(name string) Color {
	for _, c := range Colors {
		if c.Name == name {
			return c
		}
	}
	if custom.MatchString(name) {
		v, _ := strconv.ParseUint(name[1:], 16, 32)
		c := Derive([3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)})
		c.Name = name
		return c
	}
	return Colors[0]
}

// Derive picks text and accent colors for a custom background: a shade of
// the same hue, dark on light backgrounds and light on dark ones, with at
// least 4.5:1 contrast for the text.
func Derive(bg [3]uint8) Color {
	h, s, l := toHSL(bg)
	lum := luminance(bg)
	// Dark text when black contrasts better than white.
	dark := lum > 0.179
	inkL, step, accentL := 0.15, -0.01, l*0.6
	if !dark {
		inkL, step, accentL = 0.92, 0.01, l+(1-l)*0.45
	}
	inkS := math.Min(s, 0.6)
	ink := fromHSL(h, inkS, inkL)
	for contrast(lum, luminance(ink)) < 4.5 && inkL > 0 && inkL < 1 {
		inkL = math.Round((inkL+step)*100) / 100
		ink = fromHSL(h, inkS, inkL)
	}
	return Color{BG: bg, Ink: ink, Accent: fromHSL(h, s, accentL)}
}

func toHSL(c [3]uint8) (h, s, l float64) {
	r, g, b := float64(c[0])/255, float64(c[1])/255, float64(c[2])/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (hi + lo) / 2
	d := hi - lo
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch hi {
	case r:
		h = math.Mod((g-b)/d+6, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func fromHSL(h, s, l float64) [3]uint8 {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	ch := func(v float64) uint8 { return uint8(math.Round(math.Min(1, math.Max(0, v+m)) * 255)) }
	return [3]uint8{ch(r), ch(g), ch(b)}
}

// luminance is the WCAG relative luminance.
func luminance(c [3]uint8) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

func contrast(a, b float64) float64 {
	return (math.Max(a, b) + 0.05) / (math.Min(a, b) + 0.05)
}
