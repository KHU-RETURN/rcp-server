package auth

import (
	"context"

	"github.com/KHU-RETURN/rcp-server/ent"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/google/uuid"
)

// Repository는 Ent 클라이언트를 통해 유저/세션 데이터를 저장합니다.
type Repository struct {
	client *ent.Client
}

// NewRepository는 Ent 클라이언트를 주입받아 Repository를 생성합니다.
// 스키마 마이그레이션은 외부(main.go)에서 RunMigration으로 처리합니다.
func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

// UpsertUser는 이메일 기준으로 유저를 생성하거나 업데이트합니다.
// 처리 후 user.ID에 해당 유저의 UUID가 설정됩니다.
func (r *Repository) UpsertUser(ctx context.Context, user *User) error {
	u, err := r.client.User.Query().Where(entuser.EmailEQ(user.Email)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	if u != nil {
		updated, err := u.Update().SetName(user.Name).Save(ctx)
		if err != nil {
			return err
		}
		user.ID = updated.ID
		return nil
	}
	created, err := r.client.User.Create().
		SetEmail(user.Email).
		SetName(user.Name).
		Save(ctx)
	if err != nil {
		return err
	}
	user.ID = created.ID
	return nil
}

// FindByEmail은 이메일로 유저를 조회합니다. 존재하지 않으면 (nil, nil)을 반환합니다.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	u, err := r.client.User.Query().Where(entuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &User{
		ID:    u.ID,
		Email: u.Email,
		Name:  u.Name,
	}, nil
}

// CreateSession은 유저에 연결된 새 세션을 생성합니다.
func (r *Repository) CreateSession(ctx context.Context, userID uuid.UUID, session *Session) error {
	builder := r.client.Session.Create().
		SetAccessToken(session.AccessToken).
		SetRefreshToken(session.RefreshToken).
		SetExpiry(session.Expiry).
		SetProvider("GOOGLE").
		SetProviderToken(session.ProviderToken).
		SetUserID(userID)
	if session.ProviderRefresh != nil {
		builder = builder.SetProviderRefresh(*session.ProviderRefresh)
	}
	if session.ProviderExpiry != nil {
		builder = builder.SetProviderExpiry(*session.ProviderExpiry)
	}
	_, err := builder.Save(ctx)
	return err
}
