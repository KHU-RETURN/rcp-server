package compute

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

func (r *Repository) SaveInstance(ctx context.Context, ownerID uuid.UUID, inst *Instance) error {
	return r.client.Instance.Create().
		SetOwnerID(ownerID).
		SetOpenstackID(inst.OpenstackID).
		SetName(inst.Name).
		SetStatus(inst.Status).
		SetImageID(inst.ImageID).
		SetFlavorID(inst.FlavorID).
		SetKeyName(inst.KeyName).
		SetNote(inst.Note).
		SetProviderCreatedAt(inst.Created).
		Exec(ctx)
}

func (r *Repository) UpdateInstanceMetadata(ctx context.Context, ownerID uuid.UUID, openstackID string, update UpdateInstanceRequest) error {
	builder := r.client.Instance.Update().
		Where(
			entinstance.OpenstackID(openstackID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		)

	if update.Name != "" {
		builder.SetName(update.Name)
	}
	builder.SetKeyName(update.KeyName)
	builder.SetNote(update.Note)

	_, err := builder.Save(ctx)
	return err
}

func (r *Repository) DeleteByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) error {
	_, err := r.client.Instance.Delete().
		Where(
			entinstance.OpenstackID(openstackID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		).Exec(ctx)
	return err
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Instance, error) {
	rows, err := r.client.Instance.Query().
		Where(entinstance.HasOwnerWith(entuser.ID(ownerID))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]Instance, 0, len(rows))
	for _, row := range rows {
		result = append(result, entToInstance(row))
	}
	return result, nil
}

func (r *Repository) FindByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) (*Instance, error) {
	row, err := r.client.Instance.Query().
		Where(
			entinstance.OpenstackID(openstackID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	inst := entToInstance(row)
	return &inst, nil
}

func entToInstance(row *ent.Instance) Instance {
	return Instance{
		OpenstackID: row.OpenstackID,
		Name:        row.Name,
		Status:      row.Status,
		ImageID:     row.ImageID,
		FlavorID:    row.FlavorID,
		KeyName:     row.KeyName,
		Note:        row.Note,
		Created:     row.ProviderCreatedAt,
	}
}
