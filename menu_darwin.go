//go:build desktop && darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void installEditMenu(void);
*/
import "C"

// installEditMenu adds the standard macOS menu bar (App + Edit) so that
// Cut/Copy/Paste/Select All/Undo keyboard shortcuts work inside the webview.
func installEditMenu() { C.installEditMenu() }
