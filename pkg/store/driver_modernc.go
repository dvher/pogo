//go:build !android && !ios && !pogo_wasmsqlite

package store

import _ "modernc.org/sqlite"

// modernc.org/sqlite is SQLite translated to Go.
const driverName = "sqlite"

func dsn(path string) string { return path }
