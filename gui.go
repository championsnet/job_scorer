//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

// The webview/Cocoa UI must run on the main OS thread.
func init() { runtime.LockOSThread() }

// guiEnabled is true in desktop builds (go build -tags desktop).
const guiEnabled = true

// guiDefaultHome is where a double-clicked app stores its files, since Finder
// launches apps with the working directory set to "/".
func guiDefaultHome() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "JobScorer")
	}
	return ""
}

// runGUI opens the app in a native window and blocks until it is closed.
func runGUI(url, title string) {
	w := webview.New(false)
	defer w.Destroy()
	// Install the native menu bar so copy/paste keyboard shortcuts work
	// (WKWebView has no menu of its own).
	installEditMenu()
	w.SetTitle(title)
	w.SetSize(1040, 820, webview.HintNone)
	w.Navigate(url)

	// The OS pauses the webview's own JS timers when the window is in the
	// background, which can freeze the dashboard's live progress. Poke it from
	// the native side (Go timers aren't throttled) so it always stays current.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.Dispatch(func() { w.Eval("try{window.__poll&&window.__poll()}catch(e){}") })
			}
		}
	}()

	w.Run()
	close(stop)
}
