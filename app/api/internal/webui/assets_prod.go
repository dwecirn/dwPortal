//go:build production

package webui

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var embedded embed.FS

func Assets() fs.FS {
	assets, _ := fs.Sub(embedded, "static")
	return assets
}
