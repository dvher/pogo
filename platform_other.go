//go:build !linux

package main

func chooseGDKBackend() (restore func()) { return func() {} }
