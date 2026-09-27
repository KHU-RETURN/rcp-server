// function-builder compiles untrusted single-file sources in disposable Docker containers.
// Run this service under a dedicated account with access to Docker, separate from the API.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxSource = 256 << 10
const maxWasm = 32 << 20

type toolchain struct {
	image, filename string
	command         []string
}

var toolchains = map[string]toolchain{
	"rust":       {"rcp-wasm-rust:1", "main.rs", []string{"rustc", "--target", "wasm32-wasip1", "-O", "-o", "/out/function.wasm", "/src/main.rs"}},
	"go":         {"rcp-wasm-go:1", "main.go", []string{"go", "build", "-o", "/out/function.wasm", "/src/main.go"}},
	"javascript": {"rcp-wasm-javy:1", "main.js", []string{"javy", "build", "/src/main.js", "-o", "/out/function.wasm"}},
	"python":     {"python:3.11-slim", "main.py", []string{"python", "-c", "import ast; ast.parse(open('/src/main.py', encoding='utf-8').read())"}},
}

func build(ctx context.Context, language string, source []byte) ([]byte, error) {
	tc, ok := toolchains[language]
	if !ok {
		return nil, fmt.Errorf("unsupported language")
	}
	if len(source) == 0 || len(source) > maxSource {
		return nil, fmt.Errorf("invalid source size")
	}
	dir, err := os.MkdirTemp("", "rcp-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	in, out := filepath.Join(dir, "src"), filepath.Join(dir, "out")
	if err := os.Mkdir(in, 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(in, tc.filename), source, 0600); err != nil {
		return nil, err
	}
	containerName := filepath.Base(dir)
	containerUser := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	args := []string{"run", "--rm", "--name", containerName, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "1g", "--cpus", "1", "--user", containerUser, "--tmpfs", "/tmp:rw,nosuid,nodev,size=256m", "-e", "HOME=/tmp", "-e", "GOCACHE=/tmp/go-cache", "-e", "GOPATH=/tmp/go", "-v", in + ":/src:ro", "-v", out + ":/out:rw", tc.image}
	args = append(args, tc.command...)
	// #nosec G204 -- args come from fixed toolchains and internally created paths; no shell is used.
	cmd := exec.CommandContext(ctx, "docker", args...)
	output := &boundedOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	if ctx.Err() != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// #nosec G204 -- containerName is derived from os.MkdirTemp, not user input.
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", containerName).Run()
		return nil, fmt.Errorf("build timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, strings.TrimSpace(output.String()))
	}
	if language == "python" {
		path := os.Getenv("RCP_PYTHON_WASM_PATH")
		if path == "" {
			return nil, fmt.Errorf("RCP_PYTHON_WASM_PATH is required")
		}
		return readLimited(path)
	}
	return readLimited(filepath.Join(out, "function.wasm"))
}

type boundedOutput struct {
	sync.Mutex
	bytes.Buffer
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	const max = 16 << 10
	remaining := max - w.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.Buffer.Write(p[:remaining])
	}
	return len(p), nil
}

func (w *boundedOutput) String() string {
	w.Lock()
	defer w.Unlock()
	return w.Buffer.String()
}

func readLimited(path string) ([]byte, error) {
	// #nosec G703 -- callers pass only the generated output path or the configured Python runtime path.
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("WASM output must be a regular file")
	}
	// #nosec G304 G703 -- readLimited is called only for generated output or the trusted Python runtime path.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxWasm+1))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > maxWasm {
		return nil, fmt.Errorf("invalid WASM output size")
	}
	return b, nil
}

func main() {
	path := os.Getenv("RCP_FUNCTION_BUILDER_SOCKET")
	if path == "" {
		log.Fatal("RCP_FUNCTION_BUILDER_SOCKET is required")
	}
	// #nosec G703 -- the socket path is configured by the trusted systemd unit.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	// #nosec G302 G703 -- the trusted systemd socket path needs group access for the API.
	if err := os.Chmod(path, 0660); err != nil {
		log.Fatal(err)
	}
	sem := make(chan struct{}, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/build" {
			http.NotFound(w, r)
			return
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, "builder busy", http.StatusTooManyRequests)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxSource+1))
		if err != nil || len(body) > maxSource {
			http.Error(w, "source too large", http.StatusRequestEntityTooLarge)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
		defer cancel()
		wasm, err := build(ctx, r.URL.Query().Get("language"), body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/wasm")
		_, _ = w.Write(wasm)
	})
	log.Fatal((&http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}).Serve(listener))
}
