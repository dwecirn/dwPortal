VERSION ?= 0.2.0-dev
DIST := $(CURDIR)/dist

.PHONY: test build package clean validate-version
validate-version:
	@printf '%s' '$(VERSION)' | grep -Eq '^[0-9A-Za-z][0-9A-Za-z._-]*$$' || { echo 'invalid VERSION: $(VERSION)' >&2; exit 1; }

test:
	cd app/web && npm run lint && npm run build
	cd app/api && go test ./...
	python3 -m unittest discover -s script/build -p 'test_*.py'
	python3 -m unittest discover -s script/deploy -p 'test_*.py'

build: validate-version
	cd app/web && npm ci && npm run build
	mkdir -p app/api/internal/webui/static $(DIST)
	cp -R app/web/dist/. app/api/internal/webui/static/
	cd app/api && CGO_ENABLED=0 go build -tags production -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(DIST)/dwportal ./cmd/server

package: build
	rm -rf -- $(DIST)/dwportal-$(VERSION)
	mkdir -p $(DIST)/dwportal-$(VERSION)
	cp $(DIST)/dwportal deploy/dwportal.service config/config.example.yaml $(DIST)/dwportal-$(VERSION)/
	printf '%s\n' '$(VERSION)' > $(DIST)/dwportal-$(VERSION)/VERSION
	tar -C $(DIST) -czf $(DIST)/dwportal-$(VERSION)-linux-amd64.tar.gz dwportal-$(VERSION)

clean:
	rm -rf $(DIST) app/api/internal/webui/static
