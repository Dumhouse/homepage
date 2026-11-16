// Package homeservices holds assets embedded into the single binary.
package homeservices

import "embed"

// Design is the static HTML/CSS/JS prototype. During M1 it is served as-is;
// later milestones replace pages one by one with server-rendered templates.
//
//go:embed all:design
var Design embed.FS

// UI holds the server-rendered templates and the app's own static files
// (vendored htmx, ui.js) used by the real pages.
//
//go:embed templates static
var UI embed.FS
