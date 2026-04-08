package access

import (
	"context"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/keypair"
	"github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/google/uuid"
)

type keypairRepository interface {
	SaveKeyPair(ctx context.Context, userID uuid.UUID, name string) error
	DeleteKeyPair(ctx context.Context, userID uuid.UUID, name string) error
	FindNamesByUserID(ctx context.Context, userID uuid.UUID) ([]string, error)
	IsOwner(ctx context.Context, userID uuid.UUID, name string) (bool, error)
}

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) SaveKeyPair(ctx context.Context, userID uuid.UUID, name string) error {
	return r.client.KeyPair.Create().
		SetName(name).
		SetUserID(userID).
		Exec(ctx)
}

func (r *Repository) DeleteKeyPair(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := r.client.KeyPair.Delete().
		Where(
			keypair.Name(name),
			keypair.HasUserWith(user.ID(userID)),
		).
		Exec(ctx)
	return err
}

func (r *Repository) FindNamesByUserID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	keyPairs, err := r.client.KeyPair.Query().
		Where(keypair.HasUserWith(user.ID(userID))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(keyPairs))
	for _, item := range keyPairs {
		names = append(names, item.Name)
	}
	return names, nil
}

func (r *Repository) IsOwner(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	return r.client.KeyPair.Query().
		Where(
			keypair.Name(name),
			keypair.HasUserWith(user.ID(userID)),
		).
		Exist(ctx)
}
