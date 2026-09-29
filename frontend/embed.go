// Package frontend embeds the built Vite SPA (see frontend/dist, produced by
// `npm run build`) for the Go server to serve.
package frontend

import "embed"

// DistFS holds the Vite build output (frontend/dist), produced by
// `npm run build`. The committed dist/.gitkeep keeps the embed compiling
// before the UI has been built; the server then answers /ui with a notice.
//
//go:embed all:dist
var DistFS embed.FS
