# Container image for StashBert, the optional way of running it (ADR-0010,
# ADR-0014). Build with make docker.
#
# The web and Go stages run on the build platform; Go cross-compiles for
# TARGETOS/TARGETARCH, so a later multi-arch build (R03) needs no emulation.

# 1. Web UI. npm run build also runs copy:wasm (prebuild) and check:pwa
#    (postbuild).
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Static Go binary with the web UI embedded.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist/ internal/webui/dist/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/stashbert ./cmd/stashbert
# Empty data directory for the runtime image.
RUN mkdir /data

# 3. Runtime image, runs as nonroot (65532).
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/stashbert /stashbert
# A new named volume on /data takes over this owner.
COPY --from=build --chown=65532:65532 /data /data
ENV DATA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/stashbert", "-healthcheck"]
ENTRYPOINT ["/stashbert"]
