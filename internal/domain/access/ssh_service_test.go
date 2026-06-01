package access

import (
	"context"
	"errors"
	"testing"
)

type fakeSSHNotifier struct {
	gotNonce, gotCode, gotEmail string
	err                         error
}

func (f *fakeSSHNotifier) Notify(_ context.Context, nonce, code, email string) error {
	f.gotNonce = nonce
	f.gotCode = code
	f.gotEmail = email
	return f.err
}

func TestSSHService_HandleSSHCallback_Forwards(t *testing.T) {
	f := &fakeSSHNotifier{}
	s := NewSSHService(f)
	if err := s.HandleSSHCallback(context.Background(), "n1", "123456", "u@khu.ac.kr"); err != nil {
		t.Fatal(err)
	}
	if f.gotNonce != "n1" || f.gotCode != "123456" || f.gotEmail != "u@khu.ac.kr" {
		t.Fatalf("got %s/%s/%s", f.gotNonce, f.gotCode, f.gotEmail)
	}
}

func TestSSHService_RejectsEmpty(t *testing.T) {
	s := NewSSHService(&fakeSSHNotifier{})
	if err := s.HandleSSHCallback(context.Background(), "", "123456", "u@khu.ac.kr"); !errors.Is(err, ErrInvalidNonce) {
		t.Fatalf("got %v", err)
	}
	if err := s.HandleSSHCallback(context.Background(), "n", "", "u@khu.ac.kr"); !errors.Is(err, ErrInvalidSSHCode) {
		t.Fatalf("got %v", err)
	}
	if err := s.HandleSSHCallback(context.Background(), "n", "123456", ""); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("got %v", err)
	}
}

func TestSSHService_PropagatesNotifierErr(t *testing.T) {
	f := &fakeSSHNotifier{err: errors.New("boom")}
	s := NewSSHService(f)
	if err := s.HandleSSHCallback(context.Background(), "n", "123456", "u@khu.ac.kr"); err == nil {
		t.Fatal("expected error")
	}
}
