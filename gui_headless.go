//go:build !desktop

package main

// Headless build (default): no native window, pure Go, no CGO. Used for the
// server binary and Docker. The app opens in the user's browser instead.

const guiEnabled = false

func guiDefaultHome() string { return "" }

func runGUI(url, title string) {}
