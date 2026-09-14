package blockstorage

import (
	"context"

	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/ent"
	entinstance "github.com/KHU-RETURN/rcp-server/ent/instance"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) FindInstanceName(ctx context.Context, ownerID uuid.UUID, openstackID string) (string, bool, error) {
	row, err := r.client.Instance.Query().
		Where(
			entinstance.OpenstackID(openstackID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return row.Name, true, nil
}

func (r *Repository) InstanceExists(ctx context.Context, ownerID uuid.UUID, openstackID string) (bool, error) {
	_, ok, err := r.FindInstanceName(ctx, ownerID, openstackID)
	return ok, err
}
