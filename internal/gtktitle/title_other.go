//go:build !linux || !cgo

package gtktitle

import "unsafe"

// Set is a no-op outside Linux.
func Set(window unsafe.Pointer, title string) {}
