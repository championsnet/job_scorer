//go:build desktop && !darwin

package main

// Non-macOS desktop builds get standard menus/shortcuts from the OS webview.
func installEditMenu() {}
