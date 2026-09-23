// Package webui embeds the built web UI and serves it (architecture.md, 4.2).
package webui

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// dist holds the build output that make build copies from web/dist. In the
// repository it contains only .gitkeep. The all: prefix also embeds names
// starting with . or _, such as _app.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the embedded web UI with dist as its root.
func Dist() (fs.FS, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded web UI: %w", err)
	}
	return sub, nil
}

const (
	indexName = "index.html"
	// notBuilt is the body of / when fsys has no index.html.
	notBuilt = "Web-Oberfläche nicht gebaut"

	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNoCache   = "no-cache"
)

// contentTypes maps file extensions to Content-Type values, so that the
// result does not depend on the MIME tables of the operating system.
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".webmanifest": "application/manifest+json",
	".wasm":        "application/wasm",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".ico":         "image/x-icon",
	".webp":        "image/webp",
	".woff2":       "font/woff2",
	".txt":         "text/plain; charset=utf-8",
}

// handler serves the files of fsys with the SPA fallback to index.html.
type handler struct {
	fsys fs.FS
	csp  string
	// index is the content of index.html, read once so that the served page
	// always matches the hashes in csp. hasIndex is false if it is missing.
	index        []byte
	indexModTime time.Time
	hasIndex     bool
}

// Handler serves the web UI from fsys (architecture.md, 4.2 and 8):
//   - A path naming a file serves that file.
//   - A path without a file and without an extension serves index.html (SPA
//     fallback). Directories do not count as files.
//   - Everything else, /api and every path below /api/ are 404.
//   - Without index.html in fsys, / serves a short plain text page.
//
// Every response carries the security headers from architecture.md 8. The
// hashes of the inline scripts in index.html are computed once here.
func Handler(fsys fs.FS) http.Handler {
	h := &handler{fsys: fsys}
	if f, err := fsys.Open(indexName); err == nil {
		defer f.Close()
		info, statErr := f.Stat()
		data, readErr := io.ReadAll(f)
		if statErr == nil && readErr == nil && !info.IsDir() {
			h.index, h.indexModTime, h.hasIndex = data, info.ModTime(), true
		}
	}
	h.csp = contentSecurityPolicy(inlineScriptHashes(h.index))
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", h.csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")

	// path.Clean on a rooted path removes every .. element, so the name
	// cannot leave the root of fsys.
	clean := path.Clean("/" + r.URL.Path)
	// API paths never get the SPA fallback, so that a client never receives
	// HTML instead of an API error. Clean turns /api/ into /api.
	if clean == "/api" || strings.HasPrefix(clean, "/api/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(clean, "/")
	switch {
	case name == "" || name == indexName:
		h.serveIndex(w, r, name == "")
	case !fs.ValidPath(name):
		http.NotFound(w, r)
	case h.serveFile(w, r, name):
	case path.Ext(name) == "":
		h.serveIndex(w, r, false)
	default:
		http.NotFound(w, r)
	}
}

// serveIndex writes index.html. Without it, root requests get the
// placeholder text and all others 404.
func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request, root bool) {
	switch {
	case h.hasIndex:
		w.Header().Set("Content-Type", contentTypes[".html"])
		w.Header().Set("Cache-Control", cacheNoCache)
		http.ServeContent(w, r, indexName, h.indexModTime, bytes.NewReader(h.index))
	case root:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", cacheNoCache)
		// The status is already sent, so a failed write cannot be reported to the client.
		_, _ = io.WriteString(w, notBuilt)
	default:
		http.NotFound(w, r)
	}
}

// serveFile writes the file name from fsys. It reports false without
// writing anything if name does not exist, cannot be opened or is a
// directory.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	w.Header().Set("Content-Type", contentType(name))
	if cc := cacheControl(name); cc != "" {
		w.Header().Set("Cache-Control", cc)
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, info.ModTime(), rs)
		return true
	}
	// The status is already sent, so a failed copy cannot be reported to the client.
	_, _ = io.Copy(w, f)
	return true
}

// contentType returns the Content-Type for the extension of name, or
// application/octet-stream for unknown extensions.
func contentType(name string) string {
	if ct, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		return ct
	}
	return "application/octet-stream"
}

// cacheControl returns the Cache-Control value for the file name, or "" if
// the file gets no Cache-Control header. serveIndex sets it for index.html.
func cacheControl(name string) string {
	switch {
	case strings.HasPrefix(name, "assets/"), strings.HasPrefix(name, "_app/immutable/"):
		return cacheImmutable
	case name == "sw.js", name == "manifest.webmanifest":
		return cacheNoCache
	}
	return ""
}
