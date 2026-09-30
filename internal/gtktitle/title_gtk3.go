//go:build linux && cgo && gtk3

package gtktitle

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>
*/
import "C"

import "unsafe"

// Set sets the title of the GtkWindow at window. Call on the GTK main thread.
func Set(window unsafe.Pointer, title string) {
	if window == nil {
		return
	}
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	C.gtk_window_set_title((*C.GtkWindow)(window), cTitle)
}
