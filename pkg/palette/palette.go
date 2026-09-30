// Package palette lists the note colors shared by every Pogo app.
// Keep in sync with frontend/src/lib/colors.ts.
package palette

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

// Valid reports whether name is a known color.
func Valid(name string) bool {
	for _, c := range Colors {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Get returns the named color, or the default one.
func Get(name string) Color {
	for _, c := range Colors {
		if c.Name == name {
			return c
		}
	}
	return Colors[0]
}
