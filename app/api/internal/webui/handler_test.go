package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerServesRootAndSPAFallbackWithoutRedirect(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("portal index")},
		"assets/app.js": &fstest.MapFile{Data: []byte("portal script")},
	}
	handler := Handler(fs.FS(assets))

	for _, test := range []struct {
		name string
		path string
		body string
	}{
		{name: "root", path: "/", body: "portal index"},
		{name: "SPA fallback", path: "/settings/profile", body: "portal index"},
		{name: "static asset", path: "/assets/app.js", body: "portal script"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d, want 200", test.path, response.Code)
			}
			if location := response.Header().Get("Location"); location != "" {
				t.Fatalf("GET %s redirected to %q", test.path, location)
			}
			if response.Body.String() != test.body {
				t.Fatalf("GET %s body = %q, want %q", test.path, response.Body.String(), test.body)
			}
		})
	}
}
