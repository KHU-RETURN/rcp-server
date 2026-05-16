package main

import (
	"context"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent/enttest"

	_ "github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
)

func timeZero() time.Time { return time.Unix(0, 0).UTC() }

func TestRepo_ListInstancesByEmail_Empty(t *testing.T) {
	c := enttest.Open(t, "sqlite3", "file:?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	defer func() { _ = c.Close() }()
	r := newRepo(c)
	got, err := r.ListInstancesByEmail(context.Background(), "ghost@khu.ac.kr")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestRepo_ListInstancesByEmail_OnlyOwn(t *testing.T) {
	c := enttest.Open(t, "sqlite3", "file:?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	alice := c.User.Create().SetEmail("a@khu.ac.kr").SetName("a").
		SetGoogleID("a-gid").
		SetGoogleAccessToken("").SetGoogleRefreshToken("").
		SetGoogleTokenExpiry(timeZero()).SaveX(ctx)
	bob := c.User.Create().SetEmail("b@khu.ac.kr").SetName("b").
		SetGoogleID("b-gid").
		SetGoogleAccessToken("").SetGoogleRefreshToken("").
		SetGoogleTokenExpiry(timeZero()).SaveX(ctx)

	c.Instance.Create().
		SetOwnerID(alice.ID).
		SetOpenstackID("os-1").SetName("study-server").
		SetStatus("ACTIVE").SetImageID("img").SetFlavorID("fl").
		SetProviderCreatedAt(timeZero()).SaveX(ctx)
	c.Instance.Create().
		SetOwnerID(bob.ID).
		SetOpenstackID("os-2").SetName("hidden").
		SetStatus("ACTIVE").SetImageID("img").SetFlavorID("fl").
		SetProviderCreatedAt(timeZero()).SaveX(ctx)

	r := newRepo(c)
	got, err := r.ListInstancesByEmail(ctx, "a@khu.ac.kr")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "study-server" || got[0].OpenstackID != "os-1" {
		t.Fatalf("got %+v", got)
	}
}
