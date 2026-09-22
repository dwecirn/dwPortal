package catalog

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type responseService struct {
	Service
	Href string `json:"href"`
}

type response struct {
	BaseDomain string            `json:"baseDomain"`
	Services   []responseService `json:"services"`
}

func NewHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/services", func(writer http.ResponseWriter, request *http.Request) {
		config, err := store.Current()
		if err != nil {
			http.Error(writer, "configuration unavailable", http.StatusServiceUnavailable)
			return
		}

		host := forwardedValue(request.Header.Get("X-Forwarded-Host"))
		if host == "" {
			host = request.Host
		}
		hostName := stripPort(host)
		scheme := forwardedValue(request.Header.Get("X-Forwarded-Proto"))
		if scheme != "https" {
			scheme = "http"
		}
		domainAccess := hostName == config.BaseDomain || strings.HasSuffix(hostName, "."+config.BaseDomain)

		result := response{BaseDomain: config.BaseDomain, Services: make([]responseService, 0, len(config.Services))}
		for _, service := range config.Services {
			href := scheme + "://" + net.JoinHostPort(hostName, strconv.Itoa(service.Port))
			if domainAccess {
				href = scheme + "://" + service.Domain
			}
			result.Services = append(result.Services, responseService{
				Service: service,
				Href:    href,
			})
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(result)
	})
	mux.HandleFunc("GET /api/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func forwardedValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}

func stripPort(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(parsed, "[]")
	}
	return strings.Trim(host, "[]")
}
