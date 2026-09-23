package webui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// contentSecurityPolicy returns the policy from architecture.md 8 with one
// 'sha256-...' source per hash appended to script-src.
func contentSecurityPolicy(hashes []string) string {
	var scriptSrc strings.Builder
	scriptSrc.WriteString("script-src 'self' 'wasm-unsafe-eval'")
	for _, h := range hashes {
		scriptSrc.WriteString(" 'sha256-" + h + "'")
	}
	return "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; " +
		scriptSrc.String() +
		"; style-src 'self' 'unsafe-inline'; worker-src 'self'; manifest-src 'self'; frame-ancestors 'none'"
}

// inlineScriptHashes returns the base64 encoded SHA-256 of every inline
// script in page, in document order. Browsers hash the script text after
// the HTML parser has turned CRLF and lone CR into LF, so this does too.
func inlineScriptHashes(page []byte) []string {
	var hashes []string
	for _, script := range inlineScripts(page) {
		script = bytes.ReplaceAll(script, []byte("\r\n"), []byte("\n"))
		script = bytes.ReplaceAll(script, []byte("\r"), []byte("\n"))
		sum := sha256.Sum256(script)
		hashes = append(hashes, base64.StdEncoding.EncodeToString(sum[:]))
	}
	return hashes
}

// inlineScripts returns the content of every <script> element without a src
// attribute, exactly between its start tag and </script>. Tag and attribute
// names match case-insensitively, quoted attribute values may contain >, and
// HTML comments are skipped.
func inlineScripts(page []byte) [][]byte {
	var scripts [][]byte
	for i := 0; i < len(page); {
		lt := bytes.IndexByte(page[i:], '<')
		if lt < 0 {
			break
		}
		i += lt
		if bytes.HasPrefix(page[i:], []byte("<!--")) {
			end := bytes.Index(page[i+4:], []byte("-->"))
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}
		if !isTagStart(page[i:], "<script") {
			i++
			continue
		}
		hasSrc, start, ok := scanAttributes(page, i+len("<script"))
		if !ok {
			break
		}
		end := indexEndTag(page, start)
		if end < 0 {
			break
		}
		if !hasSrc {
			scripts = append(scripts, page[start:end])
		}
		i = end + len("</script")
	}
	return scripts
}

// scanAttributes reads the attributes of a start tag from page[pos:] up to
// the closing >. It reports whether one of them is src and the index after
// the >. ok is false if the tag does not end.
func scanAttributes(page []byte, pos int) (hasSrc bool, end int, ok bool) {
	for pos < len(page) {
		switch c := page[pos]; {
		case c == '>':
			return hasSrc, pos + 1, true
		case isSpace(c) || c == '/':
			pos++
		default:
			nameStart := pos
			for pos < len(page) && !isSpace(page[pos]) && page[pos] != '/' && page[pos] != '>' && page[pos] != '=' {
				pos++
			}
			if hasPrefixFold(page[nameStart:pos], "src") && pos-nameStart == len("src") {
				hasSrc = true
			}
			v := skipSpaces(page, pos)
			if v >= len(page) || page[v] != '=' {
				continue
			}
			v = skipSpaces(page, v+1)
			if v < len(page) && (page[v] == '"' || page[v] == '\'') {
				q := bytes.IndexByte(page[v+1:], page[v])
				if q < 0 {
					return false, 0, false
				}
				pos = v + 1 + q + 1
				continue
			}
			for v < len(page) && !isSpace(page[v]) && page[v] != '>' {
				v++
			}
			pos = v
		}
	}
	return false, 0, false
}

// indexEndTag returns the index of the first </script end tag in page at or
// after from, or -1.
func indexEndTag(page []byte, from int) int {
	for i := from; i < len(page); {
		j := bytes.Index(page[i:], []byte("</"))
		if j < 0 {
			return -1
		}
		i += j
		if isTagStart(page[i:], "</script") {
			return i
		}
		i += 2
	}
	return -1
}

// isTagStart reports whether b starts with the lowercase tag prefix, in any
// case, followed by whitespace, / or >.
func isTagStart(b []byte, prefix string) bool {
	if len(b) <= len(prefix) || !hasPrefixFold(b, prefix) {
		return false
	}
	c := b[len(prefix)]
	return isSpace(c) || c == '/' || c == '>'
}

// hasPrefixFold reports whether b starts with the lowercase ASCII prefix,
// ignoring ASCII case.
func hasPrefixFold(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		c := b[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

// skipSpaces returns the index of the first non-whitespace byte in page at
// or after pos.
func skipSpaces(page []byte, pos int) int {
	for pos < len(page) && isSpace(page[pos]) {
		pos++
	}
	return pos
}

// isSpace reports whether c is HTML whitespace.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}
