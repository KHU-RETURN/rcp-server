package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func setupTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:testdb?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	// modernc.org/sqlite는 DSN에 _fk=1을 지원하지 않으므로 PRAGMA로 직접 활성화합니다.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("failed to run schema migration: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		db.Close()
	})
	return client
}

func TestRepository_UpsertUser(t *testing.T) {
	client := setupTestClient(t)
	repo := NewRepository(client)
	ctx := context.Background()

	t.Run("새로운 유저를 정상적으로 저장한다", func(t *testing.T) {
		user := &User{
			Email: "test@khu.ac.kr",
			Name:  "경희인",
		}

		err := repo.UpsertUser(ctx, user)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if user.ID == uuid.Nil {
			t.Fatal("expected user.ID to be set after UpsertUser")
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
		email := "upsert-test@khu.ac.kr"

		u1 := &User{Email: email, Name: "이름1"}
		if err := repo.UpsertUser(ctx, u1); err != nil {
			t.Fatalf("failed to seed user: %v", err)
		}
		firstID := u1.ID

		u2 := &User{Email: email, Name: "이름2"}
		if err := repo.UpsertUser(ctx, u2); err != nil {
			t.Errorf("upsert failed: %v", err)
		}

		if u2.ID != firstID {
			t.Errorf("expected same ID after upsert: first=%v, second=%v", firstID, u2.ID)
		}

		saved, _ := repo.FindByEmail(ctx, email)
		if saved == nil || saved.Name != "이름2" {
			t.Errorf("expected updated name '이름2', got %v", saved)
		}
	})
}

func TestRepository_CreateSession(t *testing.T) {
	client := setupTestClient(t)
	repo := NewRepository(client)
	ctx := context.Background()

	t.Run("세션을 정상적으로 생성한다", func(t *testing.T) {
		user := &User{Email: "session@khu.ac.kr", Name: "세션유저"}
		if err := repo.UpsertUser(ctx, user); err != nil {
			t.Fatalf("UpsertUser failed: %v", err)
		}

		providerRefresh := "google-refresh"
		providerExpiry := time.Now().Add(1 * time.Hour)
		sess := &Session{
			AccessToken:     "svc-access",
			RefreshToken:    "svc-refresh",
			Expiry:          time.Now().Add(1 * time.Hour),
			Provider:        "GOOGLE",
			ProviderToken:   "google-access-token",
			ProviderRefresh: &providerRefresh,
			ProviderExpiry:  &providerExpiry,
		}

		if err := repo.CreateSession(ctx, user.ID, sess); err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		// 유저에 연결된 세션 수 확인
		count, err := client.Session.Query().Count(ctx)
		if err != nil {
			t.Fatalf("failed to count sessions: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 session, got %d", count)
		}
	})

	t.Run("ProviderRefresh가 nil인 세션을 생성한다", func(t *testing.T) {
		user := &User{Email: "norefresh@khu.ac.kr", Name: "노리프레시"}
		if err := repo.UpsertUser(ctx, user); err != nil {
			t.Fatalf("UpsertUser failed: %v", err)
		}

		sess := &Session{
			AccessToken:   "access",
			RefreshToken:  "refresh",
			Expiry:        time.Now().Add(1 * time.Hour),
			Provider:      "GOOGLE",
			ProviderToken: "google-token",
		}

		if err := repo.CreateSession(ctx, user.ID, sess); err != nil {
			t.Fatalf("CreateSession with nil ProviderRefresh failed: %v", err)
		}
	})
}

func TestRepository_FindByEmail(t *testing.T) {
	client := setupTestClient(t)
	repo := NewRepository(client)
	ctx := context.Background()

	t.Run("존재하지 않는 이메일 조회 시 nil을 반환한다", func(t *testing.T) {
		user, err := repo.FindByEmail(ctx, "non-existent@khu.ac.kr")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if user != nil {
			t.Errorf("expected nil user, got %v", user)
		}
	})
}
