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
func NewRepository(db *sql.DB) (UserRepository, error) {
	schema := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        email TEXT UNIQUE,
        name TEXT,
        access_token TEXT,       -- 우리 서비스용
        refresh_token TEXT,      -- 우리 서비스용
        expiry DATETIME,         -- 우리 서비스용 만료
        google_access_token TEXT, -- 구글 API용
        google_refresh_token TEXT,-- 구글 API용
        google_expiry DATETIME    -- 구글 토큰 만료
    );`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return &Repository{db: db}, nil
}

// UpsertUser는 Google에서 받은 정보를 DB에 저장하거나 업데이트합니다.
func (r *Repository) UpsertUser(ctx context.Context, user *User) error {
	query := `
    INSERT INTO users (
        email, name, access_token, refresh_token, expiry, 
        google_access_token, google_refresh_token, google_expiry
    )
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(email) DO UPDATE SET
        name = excluded.name,
        access_token = excluded.access_token,
        refresh_token = excluded.refresh_token,
        expiry = excluded.expiry,
        google_access_token = excluded.google_access_token,
        google_refresh_token = excluded.google_refresh_token,
        google_expiry = excluded.google_expiry;`
	var googleAccessToken, googleRefreshToken string
	var googleExpiry sql.NullTime

	if user.GoogleAuth != nil {
		googleExpiry = sql.NullTime{
			Time:  user.GoogleAuth.Expiry,
			Valid: true,
		}
	} else {
		googleExpiry = sql.NullTime{
			Valid: false,
		}
	}
	_, err := r.db.ExecContext(ctx, query,
		user.Email,
		user.Name,
		user.AccessToken,
		user.RefreshToken,
		user.Expiry,
		googleAccessToken,
		googleRefreshToken,
		googleExpiry)
	return err
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
