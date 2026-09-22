package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigListenPort(t *testing.T) {
	config, err := parseConfig([]byte("schema_version: 1\nbase_domain: dw.c\nlisten_port: 8080\nservices:\n  - id: one\n    name: One\n    domain: one.dw.c\n    port: 80\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.ListenPort == nil || *config.ListenPort != 8080 {
		t.Fatalf("ListenPort = %v, want 8080", config.ListenPort)
	}
}

func TestParseConfigRejectsInvalidListenPort(t *testing.T) {
	for _, value := range []string{"nope", "0", "65536"} {
		t.Run(value, func(t *testing.T) {
			config, err := parseConfig([]byte("schema_version: 1\nbase_domain: dw.c\nlisten_port: " + value + "\nservices:\n  - id: one\n    name: One\n    domain: one.dw.c\n    port: 80\n"))
			if err == nil {
				err = config.validate()
			}
			if err == nil || !strings.Contains(err.Error(), "listen_port") {
				t.Fatalf("expected listen_port error, got %v", err)
			}
		})
	}
}

func TestStoreReloadsValidConfigAndKeepsLastGoodValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		now := time.Now().Add(time.Second)
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}

	write("schema_version: 1\nbase_domain: dw.c\nservices:\n  - id: one\n    name: One\n    domain: one.dw.c\n    port: 80\n")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}

	write("schema_version: 1\nbase_domain: dw.c\nservices:\n  - id: two\n    name: Two\n    domain: two.dw.c\n    port: 8080\n")
	config, err := store.Current()
	if err != nil || config.Services[0].ID != "two" {
		t.Fatalf("expected hot-reloaded service, got %#v, %v", config, err)
	}

	write("not: [valid")
	config, err = store.Current()
	if err != nil || config.Services[0].ID != "two" {
		t.Fatalf("expected last good service, got %#v, %v", config, err)
	}
}
