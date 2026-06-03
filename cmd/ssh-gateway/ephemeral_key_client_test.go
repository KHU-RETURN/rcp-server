package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
)

const testAuthorizedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMz7v3R7iK4WbG2ZrM8Z8vV7n6lYx4l6Wwq8m7M+v7gL test@example"

func TestEphemeralKeyClientRegistersAndDeletesSignedKey(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/internal/ssh/ephemeral-keys" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var req access.EphemeralAuthorizedKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req.InstanceID != "server-1" || req.Username != "ubuntu" || req.AuthorizedKey != testAuthorizedKey {
			t.Fatalf("unexpected request body: %+v", req)
		}
		body, _ := json.Marshal(req)
		if !verifyHMAC([]byte("secret"), body, r.Header.Get(access.InternalSigHeader)) {
			t.Fatalf("missing or invalid signature")
		}
		seen = append(seen, r.Method)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Fatalf("unexpected method %s", r.Method)
	}))
	defer srv.Close()

	client := newEphemeralKeyClient(srv.URL, []byte("secret"))
	req := access.EphemeralAuthorizedKeyRequest{
		InstanceID:    "server-1",
		Username:      "ubuntu",
		AuthorizedKey: testAuthorizedKey,
	}
	if err := client.Register(t.Context(), req); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := client.Delete(t.Context(), req); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(seen) != 2 || seen[0] != http.MethodPost || seen[1] != http.MethodDelete {
		t.Fatalf("seen methods = %#v", seen)
	}
}
