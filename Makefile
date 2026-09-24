# Requires only GNU Make 3.81 (default on macOS).

.PHONY: generate check test run build release webui docker

# With web/package.json, the API types of the web UI are generated too.
# Without web/node_modules (fresh clone), npm ci installs the tools first.
generate:
	go generate ./...
	go tool sqlc generate
	@if [ -f web/package.json ]; then \
		(cd web && if [ ! -d node_modules ]; then npm ci; fi && npm run generate:api) || exit 1; \
	fi

check:
	@unformatted="$$(gofmt -l $$(go list -f '{{.Dir}}' ./...))" || exit 1; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test ./...
	sh -n deploy/install.sh
	@if [ -f web/package.json ]; then \
		(cd web && npm ci && npm run check && npm run lint && npm test) || exit 1; \
	fi

test:
	go test ./...

run:
	DATA_DIR=./.data go run ./cmd/stashbert

# Empties internal/webui/dist/ except for .gitkeep.
CLEAN_WEBUI_DIST = find internal/webui/dist -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +

# With web/package.json, the web UI is built and copied to
# internal/webui/dist/ (keeping .gitkeep), which the binary embeds.
webui:
	@if [ -f web/package.json ]; then \
		(cd web && npm ci && npm run build) && \
		$(CLEAN_WEBUI_DIST) && \
		cp -R web/dist/. internal/webui/dist/ || exit 1; \
	fi

build: webui
	go build -o bin/stashbert -ldflags "-X main.version=$$(git describe --tags --always --dirty)" ./cmd/stashbert

# Cross-compiles the binary for the LXC (ADR-0014) with the web UI and writes
# SHA256SUMS next to it. internal/webui/dist/ is emptied again afterwards,
# also when the Go build fails.
release: webui
	@mkdir -p bin/release
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$$(git describe --tags --always --dirty)" -o bin/release/stashbert-linux-amd64 ./cmd/stashbert; \
	status=$$?; \
	$(CLEAN_WEBUI_DIST); \
	exit $$status
	cd bin/release && \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum stashbert-linux-amd64 > SHA256SUMS; \
	else \
		shasum -a 256 stashbert-linux-amd64 > SHA256SUMS; \
	fi

# Builds the container image stashbert:dev, the optional way of running
# StashBert (ADR-0014). The Dockerfile builds the web UI itself.
docker:
	docker build --build-arg VERSION=$$(git describe --tags --always --dirty) -t stashbert:dev .
