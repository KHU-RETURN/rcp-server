package functions

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuilderFromEnvUsesDefaultSocket(t *testing.T) {
	t.Setenv("RCP_FUNCTION_BUILDER_SOCKET", "")
	builder, err := BuilderFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if builder == nil {
		t.Fatal("empty configuration disabled source builds")
	}
	// The default need not be installed on developer machines. Check where the
	// configured transport tries to connect without compiling any user code.
	transport := builder.(*SocketBuilder).client.Transport.(*http.Transport)
	conn, err := transport.DialContext(context.Background(), "tcp", "unused")
	if err == nil {
		_ = conn.Close()
		return
	}
	if !strings.Contains(err.Error(), defaultBuilderSocket) {
		t.Fatalf("unexpected socket: %v", err)
	}
}

func TestSocketBuilderUploadsSource(t *testing.T) {
	// Keep the Unix socket below platform path-length limits.
	dir, err := os.MkdirTemp("", "rcp-bld-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "b.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/build" || r.URL.Query().Get("language") != "python" || string(body) != "print(1)" {
			t.Errorf("unexpected build request: %s %s %q", r.Method, r.URL, body)
		}
		_, _ = w.Write([]byte("wasm"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	t.Setenv("RCP_FUNCTION_BUILDER_SOCKET", path)
	builder, err := BuilderFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	wasm, err := builder.Build(context.Background(), "python", []byte("print(1)"))
	if err != nil || string(wasm) != "wasm" {
		t.Fatalf("build: %q, %v", wasm, err)
	}
}

func TestMissingBuilderDoesNotPreventWasmOnlyStartup(t *testing.T) {
	t.Setenv("RCP_FUNCTION_BUILDER_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	builder, err := BuilderFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	_, err = builder.Build(context.Background(), "python", []byte("print(1)"))
	if !errors.Is(err, ErrBuildUnavailable) {
		t.Fatalf("missing builder: %v", err)
	}
}
