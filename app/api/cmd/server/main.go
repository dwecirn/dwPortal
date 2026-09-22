package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"

	"dwportal/app/api/internal/catalog"
	"dwportal/app/api/internal/webui"
)

var version = "dev"

const defaultListenPort = 6480

func main() {
	cliPort := flag.Int("port", 0, "HTTP listen port (overrides config listen_port)")
	flag.Parse()
	cliPortSet := false
	flag.Visit(func(current *flag.Flag) {
		if current.Name == "port" {
			cliPortSet = true
		}
	})
	if cliPortSet && (*cliPort < 1 || *cliPort > 65535) {
		log.Fatalf("port must be between 1 and 65535 (got %d)", *cliPort)
	}

	configPath, err := catalog.FindConfig()
	if err != nil {
		log.Fatal(err)
	}
	store, err := catalog.NewStore(configPath)
	if err != nil {
		log.Fatal(err)
	}
	config, err := store.Current()
	if err != nil {
		log.Fatal(err)
	}

	handler := http.NewServeMux()
	handler.Handle("/api/", catalog.NewHandler(store))
	handler.Handle("/", webui.Handler(webui.Assets()))
	address := ":" + strconv.Itoa(resolveListenPort(*cliPort, cliPortSet, config.ListenPort))

	log.Printf("dwPortal %s listening on %s (catalog: %s)", version, address, configPath)
	if err := http.ListenAndServe(address, handler); err != nil {
		log.Fatal(err)
	}
}

func resolveListenPort(cliPort int, cliPortSet bool, configPort *int) int {
	if cliPortSet {
		return cliPort
	}
	if configPort != nil {
		return *configPort
	}
	return defaultListenPort
}
