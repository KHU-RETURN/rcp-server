package storage

import (
	"io"
	"strings"
	"testing"
)

func TestResolveObjectContentStreamSniffsOneChunkAndPreservesBody(t *testing.T) {
	source := &chunkedReader{
		chunks: []string{"<!doctype html>", "<html><body>hello</body></html>"},
	}

	stream, contentType := resolveObjectContentStream(source, "application/octet-stream", "", "")
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html; charset=utf-8, got %q", contentType)
	}
	if source.reads != 1 {
		t.Fatalf("expected content type sniffing to read one chunk, got %d reads", source.reads)
	}

	body, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if body := string(body); body != "<!doctype html><html><body>hello</body></html>" {
		t.Fatalf("stream body was not preserved: %q", body)
	}
}

type chunkedReader struct {
	chunks []string
	reads  int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.reads >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.reads])
	r.chunks[r.reads] = r.chunks[r.reads][n:]
	if r.chunks[r.reads] == "" {
		r.reads++
	}
	return n, nil
}

func TestCanonicalObjectContentTypeStripsSmartQuotes(t *testing.T) {
	contentType, ok := canonicalObjectContentType("text/html; charset=utf-8\u201d", false)
	if !ok {
		t.Fatal("expected content type to normalize")
	}
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html; charset=utf-8, got %q", contentType)
	}
}

func TestContentTypeFromHTMLNameAddsUTF8Charset(t *testing.T) {
	contentType, ok := contentTypeFromName(strings.ToUpper("INDEX.HTML"))
	if !ok {
		t.Fatal("expected html extension to resolve")
	}
	if contentType != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html; charset=utf-8, got %q", contentType)
	}
}
