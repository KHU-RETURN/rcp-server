package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent/enttest"

	_ "github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
)

func timeZero() time.Time { return time.Unix(0, 0).UTC() }

func TestRepoFindByHost(t *testing.T) {
	c := enttest.Open(t, "sqlite3", "file:?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	user := c.User.Create().SetEmail("a@khu.ac.kr").SetName("a").
		SetGoogleID("a-gid").
		SetGoogleAccessToken("").SetGoogleRefreshToken("").
		SetGoogleTokenExpiry(timeZero()).SaveX(ctx)
	inst := c.Instance.Create().
		SetOwnerID(user.ID).
		SetOpenstackID("os-1").SetName("vm").
		SetStatus("ACTIVE").SetImageID("img").SetFlavorID("fl").
		SetProviderCreatedAt(timeZero()).SaveX(ctx)
	c.App.Create().
		SetHost("abc.apps.khu-return.com").
		SetInstance(inst).
		SaveX(ctx)

	got, err := newRepo(c).FindByHost(ctx, "abc.apps.khu-return.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Host != "abc.apps.khu-return.com" || got.OpenstackID != "os-1" {
		t.Fatalf("got %+v", got)
	}
}

func TestRepoFindByHostNotFound(t *testing.T) {
	c := enttest.Open(t, "sqlite3", "file:?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	defer func() { _ = c.Close() }()

	_, err := newRepo(c).FindByHost(context.Background(), "missing.apps.khu-return.com")
	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}
