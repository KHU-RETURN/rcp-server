package functions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const defaultBuilderSocket = "/run/rcp-function-builder/builder.sock"

type Builder interface {
	Build(context.Context, string, []byte) ([]byte, error)
}

type SocketBuilder struct{ client *http.Client }

func NewSocketBuilder(path string) *SocketBuilder {
	return &SocketBuilder{client: &http.Client{Timeout: 90 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}}
}

func (b *SocketBuilder) Build(ctx context.Context, language string, source []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://builder/build?language="+language, bytes.NewReader(source))
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBuildUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w: %s", ErrBuildUnavailable, message)
		}
		return nil, fmt.Errorf("%w: %s", ErrBuildFailed, message)
	}
	wasm, err := io.ReadAll(io.LimitReader(resp.Body, MaxWasmBytes+1))
	if err != nil {
		return nil, err
	}
	if len(wasm) > MaxWasmBytes {
		return nil, ErrInvalidWasm
	}
	return wasm, nil
}

func BuilderFromEnv() (Builder, error) {
	path := os.Getenv("RCP_FUNCTION_BUILDER_SOCKET")
	if path == "" {
		path = defaultBuilderSocket
	}
	// #nosec G703 -- this path is supplied by trusted deployment configuration.
	if _, err := os.Stat(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return NewSocketBuilder(path), nil
}
