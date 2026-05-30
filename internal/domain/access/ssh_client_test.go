package access

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rcp-ssh-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "n.sock")
}

func TestNotifyClient_PostsHMACSignedBody(t *testing.T) {
	sock := tempSock(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	gotBody := make(chan []byte, 1)
	gotSig := make(chan string, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody <- b
			gotSig <- r.Header.Get("X-RCP-Notify-Sig")
			w.WriteHeader(http.StatusOK)
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	c := NewNotifyClient(sock, []byte("secret"))
	if err := c.Notify(context.Background(), "the-nonce", "123456", "u@khu.ac.kr"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	body := <-gotBody
	sig := <-gotSig
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if sig != want {
		t.Fatalf("sig: got %q want %q", sig, want)
	}
	if string(body) != `{"nonce":"the-nonce","code":"123456","user_email":"u@khu.ac.kr"}` {
		t.Fatalf("body: %q", body)
	}
}

func TestNotifyClient_NonOKReturnsError(t *testing.T) {
	sock := tempSock(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusGone)
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	c := NewNotifyClient(sock, []byte("s"))
	err = c.Notify(context.Background(), "n", "123456", "u")
	if err == nil {
		t.Fatal("expected error")
	}
}
