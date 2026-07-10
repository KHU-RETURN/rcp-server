package storage

import (
	"context"

	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/ent"
	entcontainer "github.com/KHU-RETURN/rcp-server/ent/container"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) Save(ctx context.Context, ownerID uuid.UUID, c *Container) error {
	row, err := r.client.Container.Create().
		SetOwnerID(ownerID).
		SetOpenstackName(c.OpenstackName).
		SetName(c.Name).
		Save(ctx)
	if err != nil {
		return err
	}
	c.CreatedAt = row.CreatedAt
	return nil
}

func (r *Repository) FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error) {
	row, err := r.client.Container.Query().
		Where(
			entcontainer.Name(name),
			entcontainer.HasOwnerWith(entuser.ID(ownerID)),
		).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	c := entToContainer(row)
	return &c, nil
}

func (r *Repository) CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error) {
	return r.client.Container.Query().
		Where(entcontainer.HasOwnerWith(entuser.ID(ownerID))).
		Count(ctx)
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Container, error) {
	rows, err := r.client.Container.Query().
		Where(entcontainer.HasOwnerWith(entuser.ID(ownerID))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Container, len(rows))
	for i, row := range rows {
		result[i] = entToContainer(row)
	}
	return result, nil
}

// Delete removes the container row and reports whether a row was actually deleted.
// Returns (false, nil) when the row was already gone (concurrent delete).
func (r *Repository) Delete(ctx context.Context, ownerID uuid.UUID, name string) (bool, error) {
	n, err := r.client.Container.Delete().
		Where(
			entcontainer.Name(name),
			entcontainer.HasOwnerWith(entuser.ID(ownerID)),
		).Exec(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func entToContainer(row *ent.Container) Container {
	return Container{
		OpenstackName: row.OpenstackName,
		Name:          row.Name,
		CreatedAt:     row.CreatedAt,
	}
}
