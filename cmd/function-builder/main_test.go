package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBuildUsesIsolatedContainer(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$RCP_TEST_ARGS"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-v" ]; then
    case "$2" in *:/out:rw) dest=${2%%:/out:rw}; cp "$RCP_TEST_WASM" "$dest/function.wasm";; esac
    shift 2
  else shift; fi
done
`
	// #nosec G306 -- this test fixture must be executable by the current user.
	if err := os.WriteFile(docker, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "internal", "domain", "functions", "testdata", "echo.wasm")
	argsPath := filepath.Join(dir, "args")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RCP_TEST_WASM", fixture)
	t.Setenv("RCP_TEST_ARGS", argsPath)
	wasm, err := build(context.Background(), "rust", []byte("fn main() {}"))
	if err != nil {
		t.Fatal(err)
	}
	if len(wasm) == 0 {
		t.Fatal("missing WASM result")
	}
	// #nosec G304 -- argsPath is created under t.TempDir by this test.
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	containerUser := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	for _, flag := range []string{"--network\nnone", "--read-only", "--cap-drop\nALL", "--memory\n1g", "--user\n" + containerUser, "rcp-wasm-rust:1"} {
		if !strings.Contains(string(args), flag) {
			t.Errorf("missing container constraint %q", flag)
		}
	}
}
