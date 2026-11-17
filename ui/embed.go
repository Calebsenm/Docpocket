// Package ui embeds the built administration panel.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var build embed.FS

// FS returns the static panel rooted at the build directory.
func FS() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic("ui: embedded build directory is missing: " + err.Error())
	}
	return sub
}
