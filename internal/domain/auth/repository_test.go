package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/migrate"
	_ "modernc.org/sqlite"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))

	if err := client.Schema.Create(context.Background(), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("failed to run migration: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
		db.Close()
	})

	return NewRepository(client)
}

func TestRepository_UpsertUser(t *testing.T) {
	ctx := context.Background()

	t.Run("새로운 유저를 정상적으로 저장한다", func(t *testing.T) {
		repo := newTestRepo(t)

		user := &User{
			Email: "test@khu.ac.kr",
			Name:  "경희인",
		}

		if err := repo.UpsertUser(ctx, user); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		saved, err := repo.FindByEmail(ctx, user.Email)
		if err != nil {
			t.Fatalf("failed to find user: %v", err)
		}
		if saved == nil || saved.Name != user.Name {
			t.Errorf("saved user mismatch. expected %s, got %v", user.Name, saved)
		}
	})

	t.Run("동일한 이메일의 유저가 있으면 정보를 업데이트한다 (Upsert)", func(t *testing.T) {
		repo := newTestRepo(t)
		email := "upsert-test@khu.ac.kr"

		if err := repo.UpsertUser(ctx, &User{Email: email, Name: "이름1"}); err != nil {
			t.Fatalf("failed to seed user: %v", err)
		}

		if err := repo.UpsertUser(ctx, &User{Email: email, Name: "이름2"}); err != nil {
			t.Fatalf("upsert failed: %v", err)
		}

		saved, _ := repo.FindByEmail(ctx, email)
		if saved == nil || saved.Name != "이름2" {
			t.Errorf("expected updated name '이름2', got %v", saved)
		}
	})
}

func TestRepository_UpsertUserWithGoogleAuth(t *testing.T) {
	ctx := context.Background()

	t.Run("google tokens are stored correctly", func(t *testing.T) {
		repo := newTestRepo(t)
		googleExpiry := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)

		user := &User{
			Email: "google@khu.ac.kr",
			Name:  "구글 유저",
			GoogleAuth: &GoogleInfo{
				AccessToken:  "google-access-token",
				RefreshToken: "google-refresh-token",
				Expiry:       googleExpiry,
			},
		}

		if err := repo.UpsertUser(ctx, user); err != nil {
			t.Fatalf("UpsertUser failed: %v", err)
		}

		saved, err := repo.FindByEmail(ctx, user.Email)
		if err != nil || saved == nil {
			t.Fatalf("FindByEmail failed: %v", err)
		}
		if saved.GoogleAuth.AccessToken != "google-access-token" {
			t.Errorf("expected google_access_token=%q, got %q", "google-access-token", saved.GoogleAuth.AccessToken)
		}
		if saved.GoogleAuth.RefreshToken != "google-refresh-token" {
			t.Errorf("expected google_refresh_token=%q, got %q", "google-refresh-token", saved.GoogleAuth.RefreshToken)
		}
	})

	t.Run("google tokens are empty when GoogleAuth is nil", func(t *testing.T) {
		repo := newTestRepo(t)

		user := &User{
			Email: "nogoogle@khu.ac.kr",
			Name:  "노구글",
		}

		if err := repo.UpsertUser(ctx, user); err != nil {
			t.Fatalf("UpsertUser failed: %v", err)
		}

		saved, err := repo.FindByEmail(ctx, user.Email)
		if err != nil || saved == nil {
			t.Fatalf("FindByEmail failed: %v", err)
		}
		if saved.GoogleAuth.AccessToken != "" || saved.GoogleAuth.RefreshToken != "" {
			t.Errorf("expected empty google tokens, got access=%q refresh=%q",
				saved.GoogleAuth.AccessToken, saved.GoogleAuth.RefreshToken)
		}
	})
}

func TestRepository_FindByEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("존재하지 않는 이메일 조회 시 nil을 반환한다", func(t *testing.T) {
		repo := newTestRepo(t)

		user, err := repo.FindByEmail(ctx, "non-existent@khu.ac.kr")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if user != nil {
			t.Errorf("expected nil user, got %v", user)
		}
	})
}
