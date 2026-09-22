package catalog

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	SchemaVersion int       `yaml:"schema_version" json:"schemaVersion"`
	BaseDomain    string    `yaml:"base_domain" json:"baseDomain"`
	ListenPort    *int      `yaml:"listen_port,omitempty" json:"-"`
	Services      []Service `yaml:"services" json:"services"`
}

type Service struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Group       string `yaml:"group,omitempty" json:"group,omitempty"`
	Domain      string `yaml:"domain" json:"domain"`
	Port        int    `yaml:"port" json:"port"`
}

type fileStamp struct {
	modTime time.Time
	size    int64
}

type Store struct {
	path  string
	mu    sync.RWMutex
	value Config
	stamp fileStamp
}

func FindConfig() (string, error) {
	var candidates []string
	if configured := os.Getenv("DWPORTAL_CATALOG"); configured != "" {
		candidates = append(candidates, configured)
	} else if configured := os.Getenv("DWPORTAL_CONFIG"); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "config.yaml"))
	}
	candidates = append(candidates, "../../config/config.yaml")
	candidates = append(candidates, "catalog.yaml", "config.yaml", "config/catalog.yaml", "config/config.yaml")

	seen := make(map[string]bool)
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil || seen[absolute] {
			continue
		}
		seen[absolute] = true
		if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("catalog not found; set DWPORTAL_CATALOG or provide config/catalog.yaml")
}

func NewStore(path string) (*Store, error) {
	store := &Store{path: path}
	if err := store.reload(true); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Current() (Config, error) {
	if err := s.reload(false); err != nil {
		// A bad partial update must not interrupt requests when a valid snapshot exists.
		s.mu.RLock()
		hasSnapshot := len(s.value.Services) > 0
		s.mu.RUnlock()
		if !hasSnapshot {
			return Config{}, err
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.value), nil
}

func (s *Store) reload(force bool) error {
	info, err := os.Stat(s.path)
	if err != nil {
		return fmt.Errorf("stat config: %w", err)
	}
	nextStamp := fileStamp{modTime: info.ModTime(), size: info.Size()}

	s.mu.RLock()
	unchanged := !force && nextStamp == s.stamp
	s.mu.RUnlock()
	if unchanged {
		return nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	next, err := parseConfig(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if err := next.validate(); err != nil {
		return err
	}

	s.mu.Lock()
	s.value = next
	s.stamp = nextStamp
	s.mu.Unlock()
	return nil
}

// parseConfig intentionally accepts only dwPortal's small YAML subset: scalar
// top-level fields and a list of scalar service maps. This keeps the API
// self-contained while rejecting YAML features the shared contract does not use.
func parseConfig(data []byte) (Config, error) {
	var config Config
	var current *Service
	inServices := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if strings.ContainsRune(raw[:indent], '\t') {
			return Config{}, fmt.Errorf("line %d: tabs are not supported", lineNumber)
		}
		if indent == 0 {
			current = nil
			if trimmed == "services:" {
				inServices = true
				continue
			}
			inServices = false
			key, value, err := splitField(trimmed, lineNumber)
			if err != nil {
				return Config{}, err
			}
			if key != "base_domain" && key != "schema_version" && key != "listen_port" {
				return Config{}, fmt.Errorf("line %d: unknown top-level field %q", lineNumber, key)
			}
			text, scalarErr := scalar(value)
			err = scalarErr
			if err != nil {
				return Config{}, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			switch key {
			case "schema_version":
				config.SchemaVersion, err = strconv.Atoi(text)
			case "base_domain":
				config.BaseDomain = text
			case "listen_port":
				var port int
				port, err = strconv.Atoi(text)
				config.ListenPort = &port
			}
			if err != nil {
				return Config{}, fmt.Errorf("line %d: %s must be an integer", lineNumber, key)
			}
			continue
		}
		if !inServices {
			return Config{}, fmt.Errorf("line %d: unexpected indentation", lineNumber)
		}
		field := trimmed
		if strings.HasPrefix(field, "- ") {
			config.Services = append(config.Services, Service{})
			current = &config.Services[len(config.Services)-1]
			field = strings.TrimSpace(strings.TrimPrefix(field, "- "))
		}
		if current == nil {
			return Config{}, fmt.Errorf("line %d: service field before list item", lineNumber)
		}
		key, value, err := splitField(field, lineNumber)
		if err != nil {
			return Config{}, err
		}
		text, err := scalar(value)
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		switch key {
		case "id":
			current.ID = text
		case "name":
			current.Name = text
		case "description":
			current.Description = text
		case "group":
			current.Group = text
		case "domain":
			current.Domain = text
		case "port":
			current.Port, err = strconv.Atoi(text)
			if err != nil {
				return Config{}, fmt.Errorf("line %d: port must be an integer", lineNumber)
			}
		default:
			return Config{}, fmt.Errorf("line %d: unknown service field %q", lineNumber, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func splitField(line string, lineNumber int) (string, string, error) {
	key, value, found := strings.Cut(line, ":")
	if !found || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return "", "", fmt.Errorf("line %d: expected key: value", lineNumber)
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), nil
}

func scalar(value string) (string, error) {
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("invalid quoted string")
		}
		return decoded, nil
	}
	if strings.Contains(value, " #") {
		value, _, _ = strings.Cut(value, " #")
	}
	return strings.TrimSpace(value), nil
}

func (c *Config) validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d (expected 1)", c.SchemaVersion)
	}
	c.BaseDomain = strings.Trim(strings.ToLower(c.BaseDomain), ". ")
	if c.BaseDomain == "" {
		return errors.New("config base_domain is required")
	}
	if c.ListenPort != nil && (*c.ListenPort < 1 || *c.ListenPort > 65535) {
		return fmt.Errorf("config listen_port must be between 1 and 65535 (got %d)", *c.ListenPort)
	}
	if len(c.Services) == 0 {
		return errors.New("config must contain at least one service")
	}
	ids := make(map[string]bool)
	for i := range c.Services {
		service := &c.Services[i]
		service.ID = strings.TrimSpace(service.ID)
		service.Name = strings.TrimSpace(service.Name)
		service.Domain = strings.Trim(strings.ToLower(service.Domain), ". ")
		if service.ID == "" || service.Name == "" || service.Domain == "" {
			return fmt.Errorf("service %d requires id, name, and domain", i)
		}
		if ids[service.ID] {
			return fmt.Errorf("duplicate service id %q", service.ID)
		}
		if service.Port < 1 || service.Port > 65535 {
			return fmt.Errorf("service %q has invalid port %d", service.ID, service.Port)
		}
		ids[service.ID] = true
	}
	return nil
}

func cloneConfig(value Config) Config {
	if value.ListenPort != nil {
		listenPort := *value.ListenPort
		value.ListenPort = &listenPort
	}
	value.Services = append([]Service(nil), value.Services...)
	return value
}
