//go:build linux

package main

import "os"

// WebKitGTK renders a blank/black webview on many Linux setups (VMs, some GPUs,
// Wayland) unless its DMABUF and compositing renderers are disabled. This is the
// most common cause of a blank Wails window on Linux. Set the workarounds early,
// before the webview initialises, unless the user already set them.
func init() {
	if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}
	if os.Getenv("WEBKIT_DISABLE_COMPOSITING_MODE") == "" {
		os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	}
}
