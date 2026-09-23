package webui_test

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/schmitz-chris/stashbert/internal/webui"
)

const indexPage = `<!doctype html><html><head><title>StashBert</title></head><body></body></html>`

// wantCSP returns the policy from architecture.md 8 with the given hashes
// as 'sha256-...' sources in script-src.
func wantCSP(hashes ...string) string {
	scriptSrc := "script-src 'self' 'wasm-unsafe-eval'"
	for _, h := range hashes {
		scriptSrc += " 'sha256-" + h + "'"
	}
	return "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; " +
		scriptSrc + "; style-src 'self' 'unsafe-inline'; worker-src 'self'; manifest-src 'self'; frame-ancestors 'none'"
}

// hash returns the base64 encoded SHA-256 of s.
func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// serve sends a GET request for target to h and returns the response.
func serve(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// checkSecurityHeaders asserts the headers from architecture.md 8.
func checkSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder, csp string) {
	t.Helper()
	if got := rec.Header().Get("Content-Security-Policy"); got != csp {
		t.Errorf("Content-Security-Policy = %q, want %q", got, csp)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Errorf("Referrer-Policy = %q, want same-origin", got)
	}
}

func TestHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":              {Data: []byte(indexPage)},
		"assets/app.js":           {Data: []byte("app")},
		"_app/immutable/entry.js": {Data: []byte("entry")},
		"sw.js":                   {Data: []byte("sw")},
		"manifest.webmanifest":    {Data: []byte("{}")},
		"zxing_reader.wasm":       {Data: []byte("wasm")},
		"daten.bin":               {Data: []byte("bin")},
		"api/v2/x":                {Data: []byte("api file")},
	}
	const immutable = "public, max-age=31536000, immutable"
	tests := []struct {
		target, body, contentType, cacheControl string
		status                                  int
	}{
		{"/", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/index.html", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/vorrat", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/produkte/0190e3a0", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/assets", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/_app/immutable/", indexPage, "text/html; charset=utf-8", "no-cache", http.StatusOK},
		{"/assets/app.js", "app", "text/javascript; charset=utf-8", immutable, http.StatusOK},
		{"/_app/immutable/entry.js", "entry", "text/javascript; charset=utf-8", immutable, http.StatusOK},
		{"/sw.js", "sw", "text/javascript; charset=utf-8", "no-cache", http.StatusOK},
		{"/manifest.webmanifest", "{}", "application/manifest+json", "no-cache", http.StatusOK},
		{"/zxing_reader.wasm", "wasm", "application/wasm", "", http.StatusOK},
		{"/daten.bin", "bin", "application/octet-stream", "", http.StatusOK},
		{"/assets/fehlt.js", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/fehlt.png", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/api/v2/x", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/api/v1/unbekannt", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/api/", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/api", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
		{"/%2e%2e/api/v2", "404 page not found\n", "text/plain; charset=utf-8", "", http.StatusNotFound},
	}
	h := webui.Handler(fsys)
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := serve(h, tt.target)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if got := rec.Body.String(); got != tt.body {
				t.Errorf("body = %q, want %q", got, tt.body)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.cacheControl {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cacheControl)
			}
			checkSecurityHeaders(t, rec, wantCSP())
		})
	}
}

func TestHandlerHead(t *testing.T) {
	h := webui.Handler(fstest.MapFS{"index.html": {Data: []byte(indexPage)}})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/vorrat", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
}

func TestInlineScriptHashes(t *testing.T) {
	tests := []struct {
		name, page string
		hashes     []string
	}{
		{"one inline script", `<html><head><script>console.log(1)</script></head></html>`, []string{hash("console.log(1)")}},
		{"no script", indexPage, nil},
		{"only external scripts", `<script src="/assets/app.js"></script><script type="module" SRC='/a.js'></script>`, nil},
		{"two inline scripts", `<script>a()</script><p>x</p><script type="module">b()</script>`, []string{hash("a()"), hash("b()")}},
		{"mixed", `<script src=/a.js></script><script>c()</script>`, []string{hash("c()")}},
		{"upper case", `<SCRIPT Type="module">d()</SCRIPT >`, []string{hash("d()")}},
		{"data-src is not src", `<script data-src="x">e()</script>`, []string{hash("e()")}},
		{"quoted > in attribute", `<script nonce="a>b" type='x>y'>f()</script>`, []string{hash("f()")}},
		{"whitespace kept", "<script>\n\t\tg();\n\t</script>", []string{hash("\n\t\tg();\n\t")}},
		{"line breaks normalized", "<script>a\r\nb\rc</script>", []string{hash("a\nb\nc")}},
		{"comment skipped", `<!-- <script>h()</script> --><script>i()</script>`, []string{hash("i()")}},
		{"not a script tag", `<scripts>j()</scripts><noscript>k</noscript>`, nil},
		{"end tag in string", `<script>let s = "</scripty>"; l()</script>`, []string{hash(`let s = "</scripty>"; l()`)}},
		{"unterminated", `<script>m()`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := webui.Handler(fstest.MapFS{"index.html": {Data: []byte(tt.page)}})

			for _, target := range []string{"/", "/vorrat", "/assets/fehlt.js"} {
				checkSecurityHeaders(t, serve(h, target), wantCSP(tt.hashes...))
			}
		})
	}
}

func TestInlineScriptHashMatchesOnce(t *testing.T) {
	h := webui.Handler(fstest.MapFS{"index.html": {Data: []byte(`<script>console.log(1)</script>`)}})

	csp := serve(h, "/").Header().Get("Content-Security-Policy")

	want := "'sha256-" + hash("console.log(1)") + "'"
	if n := strings.Count(csp, "'sha256-"); n != 1 || !strings.Contains(csp, "script-src 'self' 'wasm-unsafe-eval' "+want+";") {
		t.Errorf("Content-Security-Policy = %q, want exactly one hash %s in script-src", csp, want)
	}
}

func TestHandlerWithoutIndex(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"empty":      {},
		"only asset": {"assets/app.js": {Data: []byte("app")}},
	} {
		t.Run(name, func(t *testing.T) {
			h := webui.Handler(fsys)

			rec := serve(h, "/")

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "Web-Oberfläche nicht gebaut" {
				t.Errorf("body = %q, want placeholder text", got)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
			}
			checkSecurityHeaders(t, rec, wantCSP())

			for _, target := range []string{"/vorrat", "/index.html"} {
				if rec := serve(h, target); rec.Code != http.StatusNotFound {
					t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusNotFound)
				}
			}
		})
	}
}

func TestHandlerPathTraversal(t *testing.T) {
	root := fstest.MapFS{
		"dist/index.html":    {Data: []byte(indexPage)},
		"dist/assets/app.js": {Data: []byte("app")},
		"geheim.txt":         {Data: []byte("geheim")},
		"geheim":             {Data: []byte("geheim")},
	}
	sub, err := fs.Sub(root, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	h := webui.Handler(sub)
	for _, target := range []string{
		"/../geheim.txt",
		"/../geheim",
		"/%2e%2e/geheim.txt",
		"/%2E%2E/geheim",
		"/..%2fgeheim.txt",
		"/assets/../../geheim.txt",
		"/assets/%2e%2e/%2e%2e/geheim",
		"/assets/..%2f..%2fgeheim.txt",
	} {
		t.Run(target, func(t *testing.T) {
			rec := serve(h, target)

			switch {
			case rec.Code == http.StatusNotFound:
			case rec.Code == http.StatusOK && rec.Body.String() == indexPage:
			default:
				t.Errorf("status = %d, body = %q, want 404 or the fallback", rec.Code, rec.Body.String())
			}
			checkSecurityHeaders(t, rec, wantCSP())
		})
	}
}

// noSeekFS hides the io.Seeker of the files of a fs.FS.
type noSeekFS struct{ fs.FS }

func (f noSeekFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return struct{ fs.File }{file}, nil
}

func TestHandlerWithoutSeeker(t *testing.T) {
	h := webui.Handler(noSeekFS{fstest.MapFS{
		"index.html":    {Data: []byte(`<script>n()</script>`)},
		"assets/app.js": {Data: []byte("app")},
	}})

	rec := serve(h, "/assets/app.js")

	if rec.Code != http.StatusOK || rec.Body.String() != "app" {
		t.Errorf("status = %d, body = %q, want 200 and app", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/javascript; charset=utf-8", got)
	}
	checkSecurityHeaders(t, rec, wantCSP(hash("n()")))
}

func TestDist(t *testing.T) {
	d, err := webui.Dist()
	if err != nil {
		t.Fatalf("Dist: %v", err)
	}
	if _, err := fs.Stat(d, ".gitkeep"); err != nil {
		t.Errorf("Stat .gitkeep: %v", err)
	}
	if rec := serve(webui.Handler(d), "/"); rec.Code != http.StatusOK {
		t.Errorf("status of / = %d, want %d", rec.Code, http.StatusOK)
	}
}
