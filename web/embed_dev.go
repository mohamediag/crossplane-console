//go:build !embedui

package web

import "io/fs"

// FS returns nil without the embedui build tag: local backend dev serves the
// API only, and the Vite dev server (with proxy) serves the frontend.
func FS() fs.FS { return nil }
