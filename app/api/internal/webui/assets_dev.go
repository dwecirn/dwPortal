//go:build !production

package webui

import (
	"io/fs"
	"os"
)

func Assets() fs.FS {
	directory := os.Getenv("DWPORTAL_WEB_ROOT")
	if directory == "" {
		directory = "../web/dist"
	}
	return os.DirFS(directory)
}
