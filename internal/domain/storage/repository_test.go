package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/enttest"

	_ "github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
)

func newRepoTestClient(t *testing.T) (*ent.Client, uuid.UUID) {
	t.Helper()
	c := enttest.Open(t, "sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	t.Cleanup(func() { _ = c.Close() })

	user := c.User.Create().SetEmail("storage@khu.ac.kr").SetName("storage").
		SetGoogleID("storage-gid").
		SetGoogleAccessToken("").SetGoogleRefreshToken("").
		SetGoogleTokenExpiry(time.Unix(0, 0).UTC()).SaveX(context.Background())
	return c, user.ID
}

func TestRepositorySaveDuplicateName(t *testing.T) {
	ctx := context.Background()
	c, ownerID := newRepoTestClient(t)
	repo := NewRepository(c)

	if err := repo.Save(ctx, ownerID, &Container{OpenstackName: uuid.New(), Name: "my-bucket"}); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// 같은 소유자가 같은 이름으로 다시 저장하면 (owner, name) 유니크 인덱스에 걸려
	// ErrContainerAlreadyExists로 매핑되어야 한다.
	err := repo.Save(ctx, ownerID, &Container{OpenstackName: uuid.New(), Name: "my-bucket"})
	if !errors.Is(err, ErrContainerAlreadyExists) {
		t.Fatalf("expected ErrContainerAlreadyExists, got %v", err)
	}

	// 중복 행이 없으므로 FindByName은 not-singular 없이 단일 행을 반환해야 한다.
	found, err := repo.FindByName(ctx, ownerID, "my-bucket")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if found == nil || found.Name != "my-bucket" {
		t.Fatalf("unexpected container: %+v", found)
	}
}

func TestRepositorySaveSameNameDifferentOwner(t *testing.T) {
	ctx := context.Background()
	c, ownerID := newRepoTestClient(t)
	repo := NewRepository(c)

	other := c.User.Create().SetEmail("other@khu.ac.kr").SetName("other").
		SetGoogleID("other-gid").
		SetGoogleAccessToken("").SetGoogleRefreshToken("").
		SetGoogleTokenExpiry(time.Unix(0, 0).UTC()).SaveX(ctx)

	// 유니크 인덱스는 (owner, name) 조합에만 적용되므로
	// 다른 소유자는 같은 이름의 컨테이너를 가질 수 있어야 한다.
	if err := repo.Save(ctx, ownerID, &Container{OpenstackName: uuid.New(), Name: "shared-name"}); err != nil {
		t.Fatalf("save for first owner: %v", err)
	}
	if err := repo.Save(ctx, other.ID, &Container{OpenstackName: uuid.New(), Name: "shared-name"}); err != nil {
		t.Fatalf("save for second owner: %v", err)
	}
}
