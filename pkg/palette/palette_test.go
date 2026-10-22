package palette

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/derive.json")

func TestGetAndValid(t *testing.T) {
	if !Valid(Default) || Get(Default).Name != Default {
		t.Fatalf("default color %q is not in the palette", Default)
	}
	for _, c := range Colors {
		if !Valid(c.Name) || Get(c.Name) != c {
			t.Errorf("Get(%q) = %+v", c.Name, Get(c.Name))
		}
	}
	for _, bad := range []string{"", "Yellow", "red", "#FFF176", "#fff17", "fff176", "#fff1766"} {
		if Valid(bad) {
			t.Errorf("Valid(%q)", bad)
		}
		if Get(bad).Name != Default {
			t.Errorf("Get(%q) = %q, want the default", bad, Get(bad).Name)
		}
	}
}

// TestMatchesFrontend checks that frontend/src/lib/colors.ts lists the same
// colors, in the same order, as this package.
func TestMatchesFrontend(t *testing.T) {
	src, err := os.ReadFile("../../frontend/src/lib/colors.ts")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*(\w+):\s*\{\s*bg:\s*'(#[0-9a-f]{6})',\s*ink:\s*'(#[0-9a-f]{6})',\s*accent:\s*'(#[0-9a-f]{6})'\s*\}`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	if len(matches) != len(Colors) {
		t.Fatalf("colors.ts has %d colors, palette.go has %d", len(matches), len(Colors))
	}
	hex := func(c [3]uint8) string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }
	for i, m := range matches {
		c := Colors[i]
		want := []string{c.Name, hex(c.BG), hex(c.Ink), hex(c.Accent)}
		for j, got := range m[1:] {
			if got != want[j] {
				t.Errorf("color %d: colors.ts has %v, palette.go has %v", i, m[1:], want)
				break
			}
		}
	}
}

func TestCustom(t *testing.T) {
	c := Get("#3a7bd5")
	if !Valid("#3a7bd5") || c.Name != "#3a7bd5" || c.BG != [3]uint8{0x3a, 0x7b, 0xd5} {
		t.Fatalf("Get(#3a7bd5) = %+v", c)
	}
	if Get("#000000").Ink == Get("#ffffff").Ink {
		t.Error("black and white backgrounds got the same text color")
	}
}

// TestDeriveContrast checks that derived text is readable on every
// background.
func TestDeriveContrast(t *testing.T) {
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 15 {
			for b := 0; b < 256; b += 15 {
				bg := [3]uint8{uint8(r), uint8(g), uint8(b)}
				c := Derive(bg)
				if k := contrast(luminance(bg), luminance(c.Ink)); k < 4.5 {
					t.Errorf("Derive(%v): ink %v has contrast %.2f", bg, c.Ink, k)
				}
			}
		}
	}
}

// TestDeriveFixture pins Derive to testdata/derive.json, which
// frontend/src/lib/colors.test.ts checks its own derive against.
func TestDeriveFixture(t *testing.T) {
	hex := func(c [3]uint8) string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }
	var got [][3]string
	for _, bg := range []string{
		"#ffffff", "#000000", "#808080", "#ff0000", "#00ff00", "#0000ff", "#ffff00", "#00ffff",
		"#ff00ff", "#3a7bd5", "#f4a261", "#2a9d8f", "#264653", "#e9c46a", "#e76f51", "#7f7f7f",
		"#b00b69", "#c0ffee", "#123456", "#fedcba", "#777777", "#8b0000", "#ffb6c1", "#4b0082",
	} {
		c := Get(bg)
		got = append(got, [3]string{bg, hex(c.Ink), hex(c.Accent)})
	}
	b, _ := json.MarshalIndent(got, "", "  ")
	if *update {
		if err := os.WriteFile("testdata/derive.json", append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/derive.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(b)+"\n" {
		t.Errorf("Derive changed; run go test -update and check colors.ts\ngot %s", b)
	}
}
