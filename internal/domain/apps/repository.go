package apps

import (
	"context"

	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/ent"
	entapp "github.com/KHU-RETURN/rcp-server/ent/app"
	entinstance "github.com/KHU-RETURN/rcp-server/ent/instance"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) SaveForInstance(ctx context.Context, ownerID uuid.UUID, instanceID string, app *App) (*App, error) {
	inst, err := r.client.Instance.Query().
		Where(
			entinstance.OpenstackID(instanceID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrInstanceNotFound
		}
		return nil, err
	}

	row, err := r.client.App.Create().
		SetHost(app.Host).
		SetInstance(inst).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrAppAlreadyExists
		}
		return nil, err
	}

	return entToApp(row, inst.OpenstackID), nil
}

func (r *Repository) DeleteByInstance(ctx context.Context, ownerID uuid.UUID, instanceID string) error {
	n, err := r.client.App.Delete().
		Where(
			entapp.HasInstanceWith(
				entinstance.OpenstackID(instanceID),
				entinstance.HasOwnerWith(entuser.ID(ownerID)),
			),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAppNotFound
	}
	return nil
}

func entToApp(row *ent.App, instanceID string) *App {
	return &App{
		ID:         row.ID,
		InstanceID: instanceID,
		Subdomain:  firstLabel(row.Host),
		Host:       row.Host,
	}
}
