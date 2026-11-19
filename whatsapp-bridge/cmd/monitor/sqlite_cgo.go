//go:build !windows || cgo

package main

import _ "github.com/mattn/go-sqlite3"

const sqliteDriver = "sqlite3"

func sessionDSN(path string) string { return path + "?_foreign_keys=on" }
func messageDSN(path string) string { return path + "?_busy_timeout=2000" }
