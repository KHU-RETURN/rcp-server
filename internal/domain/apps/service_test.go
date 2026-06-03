package apps

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeAppRepo struct {
	saved *App
}

func (f *fakeAppRepo) SaveForInstance(_ context.Context, _ uuid.UUID, instanceID string, app *App) (*App, error) {
	app.ID = uuid.New()
	app.InstanceID = instanceID
	f.saved = app
	return app, nil
}

func (f *fakeAppRepo) DeleteByInstance(context.Context, uuid.UUID, string) error {
	return ErrAppNotFound
}

func TestRegisterAppBuildsHostFromSubdomain(t *testing.T) {
	repo := &fakeAppRepo{}
	svc := NewService(repo)

	res, err := svc.RegisterApp(context.Background(), uuid.New(), "vm-1", RegisterAppRequest{
		Subdomain: "Abc",
	})
	if err != nil {
		t.Fatalf("RegisterApp returned error: %v", err)
	}
	if res.Host != "abc.apps.khu-return.com" {
		t.Fatalf("expected built host, got %q", res.Host)
	}
	if res.Subdomain != "abc" {
		t.Fatalf("expected normalized subdomain, got %q", res.Subdomain)
	}
	if res.InstanceID != "vm-1" {
		t.Fatalf("expected instance id vm-1, got %q", res.InstanceID)
	}
}

func TestRegisterAppRejectsEmptyHost(t *testing.T) {
	svc := NewService(&fakeAppRepo{})

	_, err := svc.RegisterApp(context.Background(), uuid.New(), "vm-1", RegisterAppRequest{
		Subdomain: " ",
	})
	if err != ErrSubdomainRequired {
		t.Fatalf("expected ErrSubdomainRequired, got %v", err)
	}
}

func TestRegisterAppRejectsDottedSubdomain(t *testing.T) {
	svc := NewService(&fakeAppRepo{})

	_, err := svc.RegisterApp(context.Background(), uuid.New(), "vm-1", RegisterAppRequest{
		Subdomain: "abc.apps.khu-return.com",
	})
	if err != ErrInvalidSubdomain {
		t.Fatalf("expected ErrInvalidSubdomain, got %v", err)
	}
}
