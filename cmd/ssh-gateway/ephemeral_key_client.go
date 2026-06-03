package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
)

type ephemeralKeyClient struct {
	baseURL string
	secret  []byte
	http    *http.Client
}

func newEphemeralKeyClient(baseURL string, secret []byte) *ephemeralKeyClient {
	return &ephemeralKeyClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:  secret,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *ephemeralKeyClient) Register(ctx context.Context, req access.EphemeralAuthorizedKeyRequest) error {
	return c.send(ctx, http.MethodPost, req, http.StatusCreated)
}

func (c *ephemeralKeyClient) Delete(ctx context.Context, req access.EphemeralAuthorizedKeyRequest) error {
	return c.send(ctx, http.MethodDelete, req, http.StatusNoContent)
}

func (c *ephemeralKeyClient) send(ctx context.Context, method string, payload access.EphemeralAuthorizedKeyRequest, wantStatus int) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+api.BasePath+"/internal/ssh/ephemeral-keys", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(access.InternalSigHeader, signHMAC(c.secret, body))
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ephemeral key %s: %w", strings.ToLower(method), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ephemeral key %s: status %d, body=%q", strings.ToLower(method), resp.StatusCode, string(b))
	}
	return nil
}

func signHMAC(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
