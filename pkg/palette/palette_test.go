package palette

import (
	"fmt"
	"os"
	"regexp"
	"testing"
)

func TestGetAndValid(t *testing.T) {
	if !Valid(Default) || Get(Default).Name != Default {
		t.Fatalf("default color %q is not in the palette", Default)
	}
	for _, c := range Colors {
		if !Valid(c.Name) || Get(c.Name) != c {
			t.Errorf("Get(%q) = %+v", c.Name, Get(c.Name))
		}
	}
	for _, bad := range []string{"", "Yellow", "red", "#fff176"} {
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
