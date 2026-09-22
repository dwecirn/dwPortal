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
		if name == "" || name == "." {
			r.URL.Path = "/"
		} else if _, err := fs.Stat(assets, name); err != nil {
			// Let FileServer serve index.html through the directory root. Requesting
			// /index.html directly triggers its canonical redirect back to ./.
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
