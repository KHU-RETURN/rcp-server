package ssh

import (
	"context"
	"errors"
	"testing"
)

type fakeNotifier struct {
	gotNonce, gotEmail string
	err                error
}

func (f *fakeNotifier) Notify(_ context.Context, nonce, email string) error {
	f.gotNonce = nonce
	f.gotEmail = email
	return f.err
}

func TestService_HandleSSHCallback_Forwards(t *testing.T) {
	f := &fakeNotifier{}
	s := NewService(f)
	if err := s.HandleSSHCallback(context.Background(), "n1", "u@khu.ac.kr"); err != nil {
		t.Fatal(err)
	}
	if f.gotNonce != "n1" || f.gotEmail != "u@khu.ac.kr" {
		t.Fatalf("got %s/%s", f.gotNonce, f.gotEmail)
	}
}

func TestService_RejectsEmpty(t *testing.T) {
	s := NewService(&fakeNotifier{})
	if err := s.HandleSSHCallback(context.Background(), "", "u@khu.ac.kr"); !errors.Is(err, ErrInvalidNonce) {
		t.Fatalf("got %v", err)
	}
	if err := s.HandleSSHCallback(context.Background(), "n", ""); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("got %v", err)
	}
}

func TestService_PropagatesNotifierErr(t *testing.T) {
	f := &fakeNotifier{err: errors.New("boom")}
	s := NewService(f)
	if err := s.HandleSSHCallback(context.Background(), "n", "u@khu.ac.kr"); err == nil {
		t.Fatal("expected error")
	}
}
