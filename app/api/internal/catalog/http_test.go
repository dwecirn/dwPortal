package catalog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServicesResolveDomainAndIPTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "schema_version: 1\nbase_domain: dw.c\nservices:\n  - id: git\n    name: Git\n    domain: git.dw.c\n    port: 3000\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store)

	for _, test := range []struct {
		host string
		want string
	}{
		{"dw.c", "http://git.dw.c"},
		{"192.168.1.20:8080", "http://192.168.1.20:3000"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/services", nil)
		request.Host = test.host
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.want) {
			t.Fatalf("host %s: status %d body %s", test.host, recorder.Code, recorder.Body.String())
		}
	}
}
