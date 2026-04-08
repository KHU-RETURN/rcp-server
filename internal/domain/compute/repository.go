package compute

import (
	"context"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/instance"
	"github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/google/uuid"
)

type computeRepository interface {
	SaveInstance(ctx context.Context, userID uuid.UUID, openstackID, name string) error
	DeleteInstance(ctx context.Context, userID uuid.UUID, openstackID string) error
	FindOpenstackIDsByUserID(ctx context.Context, userID uuid.UUID) (map[string]string, error)
	IsOwner(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error)
}

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) SaveInstance(ctx context.Context, userID uuid.UUID, openstackID, name string) error {
	return r.client.Instance.Create().
		SetOpenstackID(openstackID).
		SetName(name).
		SetUserID(userID).
		Exec(ctx)
}

func (r *Repository) DeleteInstance(ctx context.Context, userID uuid.UUID, openstackID string) error {
	_, err := r.client.Instance.Delete().
		Where(
			instance.OpenstackID(openstackID),
			instance.HasUserWith(user.ID(userID)),
		).
		Exec(ctx)
	return err
}

func (r *Repository) FindOpenstackIDsByUserID(ctx context.Context, userID uuid.UUID) (map[string]string, error) {
	instances, err := r.client.Instance.Query().
		Where(instance.HasUserWith(user.ID(userID))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	openstackIDs := make(map[string]string, len(instances))
	for _, item := range instances {
		openstackIDs[item.OpenstackID] = item.Name
	}
	return openstackIDs, nil
}

func (r *Repository) IsOwner(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
	return r.client.Instance.Query().
		Where(
			instance.OpenstackID(openstackID),
			instance.HasUserWith(user.ID(userID)),
		).
		Exist(ctx)
}
