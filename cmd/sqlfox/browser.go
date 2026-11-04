package main

// openUI launches the SqlFox interface.
// If preferAppMode is true, it attempts to launch a standalone frameless native app window
// using Chromium, Google Chrome, Microsoft Edge, or Brave.
// If no supported Chromium browser is found or if app launch fails, it gracefully falls back
// to the system's default web browser.
func openUI(targetURL string, preferAppMode bool) {
	if preferAppMode {
		if err := launchAppWindow(targetURL); err == nil {
			return
		}
	}

	_ = openDefaultBrowser(targetURL)
}

// openBrowser maintains backward compatibility for existing callers.
func openBrowser(targetURL string) {
	openUI(targetURL, true)
}
