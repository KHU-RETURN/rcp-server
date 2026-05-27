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
// User.CurrentRefreshJTI가 non-nil이면 함께 저장(회전), nil이면 컬럼 변경 없음.
func (r *Repository) UpsertUser(ctx context.Context, user *User) error {
	var googleAccessToken, googleRefreshToken string
	var googleExpiry time.Time

	if user.GoogleAuth != nil {
		googleAccessToken = user.GoogleAuth.AccessToken
		googleRefreshToken = user.GoogleAuth.RefreshToken
		googleExpiry = user.GoogleAuth.Expiry
	}

	create := r.db.User.Create().
		SetEmail(user.Email).
		SetName(user.Name).
		SetGoogleID(user.GoogleID).
		SetGoogleAccessToken(googleAccessToken).
		SetGoogleRefreshToken(googleRefreshToken).
		SetGoogleTokenExpiry(googleExpiry)
	if user.CurrentRefreshJTI != nil {
		create = create.SetCurrentRefreshJti(*user.CurrentRefreshJTI)
	}

	err := create.
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
			if user.CurrentRefreshJTI != nil {
				u.SetCurrentRefreshJti(*user.CurrentRefreshJTI)
			}
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
		ID:                u.ID,
		Email:             u.Email,
		Name:              u.Name,
		GoogleID:          u.GoogleID,
		CurrentRefreshJTI: u.CurrentRefreshJti,
		GoogleAuth: &GoogleInfo{
			AccessToken:  u.GoogleAccessToken,
			RefreshToken: u.GoogleRefreshToken,
			Expiry:       u.GoogleTokenExpiry,
		},
	}, nil
}

// SetRefreshJTI는 user의 활성 refresh token jti를 갱신합니다. jti가 nil이면 컬럼을 비웁니다(logout).
func (r *Repository) SetRefreshJTI(ctx context.Context, email string, jti *string) error {
	update := r.db.User.Update().Where(entuser.Email(email))
	if jti == nil {
		update = update.ClearCurrentRefreshJti()
	} else {
		update = update.SetCurrentRefreshJti(*jti)
	}
	if _, err := update.Save(ctx); err != nil {
		return fmt.Errorf("failed to set refresh jti: %w", err)
	}
	return nil
}

// RotateRefreshJTI는 저장된 jti가 oldJTI와 일치할 때만 newJTI로 교체합니다(compare-and-set).
// 같은 refresh token으로 동시에 회전을 시도해도 단 하나만 성공합니다.
// 반환된 bool은 실제로 회전이 일어났는지(=요청자가 승자였는지)를 나타냅니다.
func (r *Repository) RotateRefreshJTI(ctx context.Context, email string, oldJTI, newJTI string) (bool, error) {
	n, err := r.db.User.Update().
		Where(
			entuser.Email(email),
			entuser.CurrentRefreshJti(oldJTI),
		).
		SetCurrentRefreshJti(newJTI).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to rotate refresh jti: %w", err)
	}
	return n == 1, nil
}
