package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNotifyHandler_HMACOK(t *testing.T) {
	store := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := store.New()
	h := newNotifyHandler(store, []byte("secret"))

	body := []byte(`{"nonce":"` + p.Nonce + `","code":"` + p.Code + `","user_email":"a@khu.ac.kr"}`)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-RCP-Notify-Sig", sign([]byte("secret"), body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		got, _ := io.ReadAll(rr.Body)
		t.Fatalf("status %d, body %s", rr.Code, got)
	}
	email, err := p.Wait(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if email != "a@khu.ac.kr" {
		t.Fatalf("got email %q", email)
	}
}

func TestNotifyHandler_HMACMismatch(t *testing.T) {
	store := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := store.New()
	h := newNotifyHandler(store, []byte("secret"))
	body := []byte(`{"nonce":"` + p.Nonce + `","code":"` + p.Code + `","user_email":"a@khu.ac.kr"}`)

	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-RCP-Notify-Sig", sign([]byte("WRONG"), body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestNotifyHandler_CodeMismatch(t *testing.T) {
	store := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := store.New()
	h := newNotifyHandler(store, []byte("secret"))
	body := []byte(`{"nonce":"` + p.Nonce + `","code":"000000","user_email":"a@khu.ac.kr"}`)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-RCP-Notify-Sig", sign([]byte("secret"), body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestNotifyHandler_UnknownNonce(t *testing.T) {
	store := newSessionStore(time.Minute, defaultMaxPendingSessions)
	h := newNotifyHandler(store, []byte("secret"))
	body := []byte(`{"nonce":"missing","code":"123456","user_email":"a@khu.ac.kr"}`)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-RCP-Notify-Sig", sign([]byte("secret"), body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", rr.Code)
	}
}

func TestNotifyHandler_RejectsNonPOST(t *testing.T) {
	h := newNotifyHandler(newSessionStore(time.Minute, defaultMaxPendingSessions), []byte("s"))
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "/notify", strings.NewReader(""))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", m, rr.Code)
		}
	}
}

func TestNotifyHandler_BadJSON(t *testing.T) {
	store := newSessionStore(time.Minute, defaultMaxPendingSessions)
	h := newNotifyHandler(store, []byte("s"))
	body := []byte(`{not json`)
	req := httptest.NewRequest(http.MethodPost, "/notify", bytes.NewReader(body))
	req.Header.Set("X-RCP-Notify-Sig", sign([]byte("s"), body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
