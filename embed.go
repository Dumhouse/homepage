// Package homeservices holds assets embedded into the single binary.
package homeservices

import "embed"

// Assets holds the shared stylesheet and boot scripts served under /assets/.
//
//go:embed assets
var Assets embed.FS

// UI holds the server-rendered templates and the app's own static files
// (vendored htmx, ui.js) used by the real pages.
//
//go:embed templates static
var UI embed.FS
