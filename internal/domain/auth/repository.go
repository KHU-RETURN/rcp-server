package auth

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

// Repository는 *ent.Client를 통해 유저 데이터를 저장합니다.
type Repository struct {
	db *ent.Client
}

// NewRepository는 ent 클라이언트를 주입받습니다.
func NewRepository(db *ent.Client) *Repository {
	return &Repository{db: db}
}

// UpsertUser는 Google에서 받은 정보를 DB에 저장하거나 업데이트합니다.
func (r *Repository) UpsertUser(ctx context.Context, user *User) error {
	var googleAccessToken, googleRefreshToken string
	var googleExpiry time.Time

	if user.GoogleAuth != nil {
		googleAccessToken = user.GoogleAuth.AccessToken
		googleRefreshToken = user.GoogleAuth.RefreshToken
		googleExpiry = user.GoogleAuth.Expiry
	}

	err := r.db.User.Create().
		SetEmail(user.Email).
		SetName(user.Name).
		SetGoogleID(user.GoogleID).
		SetGoogleAccessToken(googleAccessToken).
		SetGoogleRefreshToken(googleRefreshToken).
		SetGoogleTokenExpiry(googleExpiry).
		OnConflict(
			sql.ConflictColumns(entuser.FieldEmail),
		).
		Update(func(u *ent.UserUpsert) {
			u.SetName(user.Name)
			u.SetGoogleID(user.GoogleID)
			u.SetGoogleAccessToken(googleAccessToken)
			u.SetGoogleRefreshToken(googleRefreshToken)
			u.SetGoogleTokenExpiry(googleExpiry)
			u.SetUpdatedAt(time.Now())
		}).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to upsert user: %w", err)
	}
	return nil
}

// FindByEmail은 이메일로 기존 유저를 조회합니다.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	u, err := r.db.User.Query().
		Where(entuser.Email(email)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return &User{
		ID:       u.ID,
		Email:    u.Email,
		Name:     u.Name,
		GoogleID: u.GoogleID,
		GoogleAuth: &GoogleInfo{
			AccessToken:  u.GoogleAccessToken,
			RefreshToken: u.GoogleRefreshToken,
			Expiry:       u.GoogleTokenExpiry,
		},
	}, nil
}
