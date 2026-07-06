//go:build embedui

// Package web embeds the built SPA. The embedui build tag keeps plain
// `go build` / `go test` working without a frontend build; the Docker build
// compiles with -tags embedui after `npm run build` has produced dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend, or nil if unavailable.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
