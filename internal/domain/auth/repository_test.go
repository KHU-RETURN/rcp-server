package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버
)

func TestRepository_UpsertUser(t *testing.T) {
	// 1. 드라이버 이름을 "sqlite"로 변경 (3을 뺍니다)
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	repo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	ctx := context.Background()

	t.Run("새로운 유저를 정상적으로 저장한다", func(t *testing.T) {
		user := &User{
			Email:        "test@khu.ac.kr",
			Name:         "경희인",
			AccessToken:  "access-123",
			RefreshToken: "refresh-123",
			Expiry:       time.Now().Add(1 * time.Hour),
		}

		err := repo.UpsertUser(ctx, user)
		if err != nil {
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
		email := "upsert-test@khu.ac.kr"

		u1 := &User{Email: email, Name: "이름1"}
		if err := repo.UpsertUser(ctx, u1); err != nil {
			t.Fatalf("failed to seed user: %v", err)
		}

		u2 := &User{Email: email, Name: "이름2"}
		err := repo.UpsertUser(ctx, u2)
		if err != nil {
			t.Errorf("upsert failed: %v", err)
		}

		saved, _ := repo.FindByEmail(ctx, email)
		if saved == nil || saved.Name != "이름2" {
			t.Errorf("expected updated name '이름2', got %v", saved)
		}
	})
}

func TestRepository_FindByEmail(t *testing.T) {
	// 여기도 "sqlite"로 변경하고 에러 체크 추가
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}()

	repo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
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
