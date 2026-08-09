// Package web embeds the built Svelte UI so the whole ERPX mock ships as one
// binary. Run `npm --prefix ui run build` to populate web/dist.
package web

import (
	"embed"
	"errors"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built UI rooted at dist/, or an error when the bundle has
// not been built yet.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, errors.New("web/dist/index.html missing: run `npm --prefix ui run build`")
	}
	return sub, nil
}
