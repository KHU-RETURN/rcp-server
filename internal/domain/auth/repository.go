package auth

import (
	"context"
	"database/sql"
	"fmt"
)

type Repository struct {
	db *sql.DB
}

type UserRepository interface {
	UpsertUser(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
}

// NewRepository는 DB 연결을 주입받고 초기 테이블을 생성합니다.
func NewRepository(db *sql.DB) UserRepository {
	// 메모리 DB이므로 실행 시마다 테이블 생성 필요
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		name TEXT,
		access_token TEXT,
		refresh_token TEXT,
		expiry DATETIME
	);`

	_, err := db.Exec(schema)
	if err != nil {
		fmt.Printf("SQLite 테이블 생성 실패: %v\n", err)
	}

	return &Repository{db: db}
}

// UpsertUser는 Google에서 받은 정보를 DB에 저장하거나 업데이트합니다.
func (r *Repository) UpsertUser(ctx context.Context, user *User) error {
	// 구조체 변수명에 맞춘 쿼리
	query := `
	INSERT INTO users (email, name, access_token, refresh_token, expiry)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(email) DO UPDATE SET
		name = excluded.name,
		access_token = excluded.access_token,
		refresh_token = excluded.refresh_token,
		expiry = excluded.expiry;`

	_, err := r.db.ExecContext(ctx, query,
		user.Email,
		user.Name,
		user.AccessToken,
		user.RefreshToken,
		user.Expiry,
	)

	if err != nil {
		return fmt.Errorf("유저 저장 실패 (Upsert): %w", err)
	}

	return nil
}

// FindByEmail은 이메일로 기존 유저를 조회합니다.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, name, access_token, refresh_token, expiry FROM users WHERE email = ?`

	row := r.db.QueryRowContext(ctx, query, email)

	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AccessToken, &u.RefreshToken, &u.Expiry)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // 찾는 유저 없음
		}
		return nil, err
	}

	return u, nil
}
