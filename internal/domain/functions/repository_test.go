package functions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/enttest"
	_ "github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
)

func TestRepositoryScopesFunctionsToOwner(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	t.Cleanup(func() { _ = client.Close() })
	createUser := func(email string) *ent.User {
		return client.User.Create().SetEmail(email).SetName(email).SetGoogleID(email).
			SetGoogleAccessToken("").SetGoogleRefreshToken("").
			SetGoogleTokenExpiry(time.Unix(0, 0)).SaveX(ctx)
	}
	a := createUser("a@khu.ac.kr")
	b := createUser("b@khu.ac.kr")
	repo := NewRepository(client)
	fn, err := repo.Create(ctx, a.ID, "echo", echoWasm)
	if err != nil {
		t.Fatal(err)
	}
	public, err := repo.GetPublic(ctx, fn.ID)
	if err != nil || public.OwnerID != a.ID {
		t.Fatalf("public lookup lost function owner: %+v, %v", public, err)
	}
	listed, err := repo.List(ctx, a.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != fn.ID || len(listed[0].Wasm) != 0 {
		t.Fatalf("list should return metadata only: %+v, %v", listed, err)
	}
	if _, err := repo.Create(ctx, a.ID, "echo", echoWasm); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name should conflict: %v", err)
	}
	if _, err := repo.Create(ctx, b.ID, "echo", echoWasm); err != nil {
		t.Fatalf("different owner may use same name: %v", err)
	}
	if _, err := repo.Get(ctx, b.ID, fn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read should fail: %v", err)
	}
	hash := make([]byte, 32)
	hash[0] = 42
	created := time.Now().UTC().Truncate(time.Second)
	expires := created.Add(24 * time.Hour)
	if _, err := repo.SetKey(ctx, b.ID, fn.ID, hash, created, expires); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner key update should fail: %v", err)
	}
	if _, err := repo.SetKey(ctx, a.ID, fn.ID, hash, created, expires); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetPublic(ctx, fn.ID)
	if err != nil || !stored.KeyEnabled || !stored.KeyExpiresAt.Equal(expires) || stored.KeyHash[0] != 42 {
		t.Fatalf("key metadata was not persisted: %+v, %v", stored, err)
	}
	if _, err := repo.DeleteKey(ctx, a.ID, fn.ID); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetPublic(ctx, fn.ID)
	if err != nil || stored.KeyEnabled || stored.KeyCreatedAt != nil || stored.KeyExpiresAt != nil {
		t.Fatalf("key metadata was not cleared: %+v, %v", stored, err)
	}
	if _, err := repo.Update(ctx, b.ID, fn.ID, echoWasm); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner update should fail: %v", err)
	}
	if err := repo.Delete(ctx, b.ID, fn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner delete should fail: %v", err)
	}
	if err := repo.Delete(ctx, a.ID, fn.ID); err != nil {
		t.Fatal(err)
	}
}
