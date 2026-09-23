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
	DATA_DIR=./.data COOKIE_SECURE=false go run ./cmd/stashbert

build:
	go build -o bin/stashbert -ldflags "-X main.version=$$(git describe --tags --always --dirty)" ./cmd/stashbert
