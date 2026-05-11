package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
)

type notifyHandler struct {
	store  *sessionStore
	secret []byte
}

func newNotifyHandler(store *sessionStore, secret []byte) *notifyHandler {
	return &notifyHandler{store: store, secret: secret}
}

func (h *notifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != access.NotifyPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	got := r.Header.Get(access.NotifySigHeader)
	if !verifyHMAC(h.secret, body, got) {
		http.Error(w, "hmac", http.StatusUnauthorized)
		return
	}
	var b access.NotifyRequest
	if err := json.Unmarshal(body, &b); err != nil || b.Nonce == "" || b.UserEmail == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := h.store.Resolve(b.Nonce, b.UserEmail); err != nil {
		if errors.Is(err, ErrNonceUnknown) {
			http.Error(w, "gone", http.StatusGone)
			return
		}
		http.Error(w, "resolve", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func verifyHMAC(secret, body []byte, gotHex string) bool {
	want, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// listenNotifySocket creates the Unix socket with 0o660 so group rcp peers can connect.
func listenNotifySocket(path string, log *slog.Logger) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	_ = os.Remove(path) // stale socket from a prior crash
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil { //nolint:gosec // G302: peer access via group rcp
		_ = ln.Close()
		return nil, err
	}
	log.Info("notify socket listening", "path", path)
	return ln, nil
}

// runNotifyServer blocks until ctx is cancelled or the underlying server exits.
func runNotifyServer(ctx context.Context, ln net.Listener, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	log.Debug("notify server started")
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-errCh
		log.Debug("notify server stopped")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}
