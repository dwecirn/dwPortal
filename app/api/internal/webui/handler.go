package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func Handler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." {
			name = "index.html"
		}
		if _, err := fs.Stat(assets, name); err != nil {
			r.URL.Path = "/index.html"
		}
		files.ServeHTTP(w, r)
	})
}
