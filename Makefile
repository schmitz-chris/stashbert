# Requires only GNU Make 3.81 (default on macOS).

.PHONY: generate check test run build

generate:
	go generate ./...
	go tool sqlc generate

check:
	@unformatted="$$(gofmt -l $$(go list -f '{{.Dir}}' ./...))" || exit 1; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	go vet ./...
	go test ./...

test:
	go test ./...

run:
	DATA_DIR=./.data go run ./cmd/stashbert

# With web/package.json, the web UI is built first and copied to
# internal/webui/dist/ (keeping .gitkeep), which the binary embeds.
build:
	@if [ -f web/package.json ]; then \
		(cd web && npm ci && npm run build) && \
		find internal/webui/dist -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} + && \
		cp -R web/dist/. internal/webui/dist/ || exit 1; \
	fi
	go build -o bin/stashbert -ldflags "-X main.version=$$(git describe --tags --always --dirty)" ./cmd/stashbert
