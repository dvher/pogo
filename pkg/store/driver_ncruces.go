//go:build android || ios || pogo_wasmsqlite

package store

import _ "github.com/ncruces/go-sqlite3/driver"

// On phones SQLite runs as WebAssembly (ncruces/go-sqlite3), which does its
// file I/O through Go's os package. modernc.org/libc makes raw legacy system
// calls (such as lstat) that Android's seccomp filter kills on x86_64.
const driverName = "sqlite3"

func dsn(path string) string { return "file:" + path }
