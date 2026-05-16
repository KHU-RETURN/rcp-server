package access

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// NotifyClient posts session-resolution notifications to ssh-gateway over its
// Unix socket, signed with HMAC-SHA256.
type NotifyClient struct {
	sockPath string
	secret   []byte
	http     *http.Client
}

func NewNotifyClient(sockPath string, secret []byte) *NotifyClient {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		},
	}
	return &NotifyClient{
		sockPath: sockPath,
		secret:   secret,
		http:     &http.Client{Transport: tr, Timeout: 5 * time.Second},
	}
}

func (c *NotifyClient) Notify(ctx context.Context, nonce, userEmail string) error {
	body, err := json.Marshal(NotifyRequest{Nonce: nonce, UserEmail: userEmail})
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+NotifyPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(NotifySigHeader, sig)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("notify gateway: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("notify gateway: status %d, body=%q", resp.StatusCode, string(b))
	}
	return nil
}
