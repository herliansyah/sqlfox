package web

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var distFS embed.FS

// Assets returns the embedded static web filesystem rooted at dist.
func Assets() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}
