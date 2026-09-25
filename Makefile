# Requires only GNU Make 3.81 (default on macOS).

.PHONY: generate check test run build release webui docker

# Shell scripts that run in the LXC (ADR-0014, ADR-0019).
SCRIPTS = deploy/install.sh deploy/stashbert-update deploy/stashbert-restore

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
	for f in $(SCRIPTS); do sh -n "$$f" || exit 1; done
	@if command -v shellcheck >/dev/null 2>&1; then \
		echo "shellcheck $(SCRIPTS)"; \
		shellcheck $(SCRIPTS) || exit 1; \
	else \
		echo "shellcheck not installed, skipped"; \
	fi
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

# Version for make release, by default from git describe; the release
# workflow passes the tag (ADR-0019).
VERSION ?= $(shell git describe --tags --always --dirty)
# Name of the release archive and of the folder in it, with VERSION without
# its leading v: stashbert_<version>_linux_amd64.
RELEASE_NAME = stashbert_$(patsubst v%,%,$(VERSION))_linux_amd64
RELEASE_DIR = bin/release/$(RELEASE_NAME)
# Archive entries belong to root:root. GNU tar (CI) and bsdtar (macOS) name
# the options differently.
TAR_OWNER = $$(if tar --version 2>/dev/null | grep -q 'GNU tar'; then echo --owner=0 --group=0; else echo --uid 0 --gid 0; fi)

# Builds the release package for the LXC (ADR-0019) in bin/release/, which is
# emptied first: the archive RELEASE_NAME.tar.gz with a folder of the same name
# (binary cross-compiled with the web UI, install.sh, unit, env example,
# stashbert-update and stashbert-restore), stashbert-update on its own for
# the first installation, and SHA256SUMS over all files. internal/webui/dist/
# is emptied again after the Go build, also when it fails. On macOS,
# COPYFILE_DISABLE=1 and --no-xattrs keep ._ files and extended attributes
# out of the archive.
release: webui
	rm -rf bin/release
	mkdir -p $(RELEASE_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(RELEASE_DIR)/stashbert ./cmd/stashbert; \
	status=$$?; \
	$(CLEAN_WEBUI_DIST); \
	exit $$status
	cp $(SCRIPTS) deploy/stashbert.service deploy/stashbert.env.example $(RELEASE_DIR)/
	chmod 0755 $(RELEASE_DIR) $(RELEASE_DIR)/stashbert $(RELEASE_DIR)/install.sh $(RELEASE_DIR)/stashbert-update $(RELEASE_DIR)/stashbert-restore
	chmod 0644 $(RELEASE_DIR)/stashbert.service $(RELEASE_DIR)/stashbert.env.example
	cd bin/release && COPYFILE_DISABLE=1 tar $(TAR_OWNER) --numeric-owner --no-xattrs -czf $(RELEASE_NAME).tar.gz $(RELEASE_NAME)
	rm -rf $(RELEASE_DIR)
	cp deploy/stashbert-update bin/release/stashbert-update
	chmod 0755 bin/release/stashbert-update
	cd bin/release && \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum * > SHA256SUMS; \
	else \
		shasum -a 256 * > SHA256SUMS; \
	fi

# Builds the container image stashbert:dev, the optional way of running
# StashBert (ADR-0014). The Dockerfile builds the web UI itself.
docker:
	docker build --build-arg VERSION=$$(git describe --tags --always --dirty) -t stashbert:dev .
