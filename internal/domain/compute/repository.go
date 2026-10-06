package compute

import (
	"context"
	"encoding/json"
	"strings"

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
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	builder := tx.Instance.Update().
		Where(
			entinstance.OpenstackID(openstackID),
			entinstance.HasOwnerWith(entuser.ID(ownerID)),
		)

	if update.Name != "" {
		builder.SetName(update.Name)
	}
	builder.SetKeyName(update.KeyName)
	builder.SetNote(update.Note)

	n, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		payload, _ := json.Marshal(map[string]string{"kind": "instance", "resource_id": openstackID, "reason": "metadata changed"})
		if _, err = tx.OutboxEvent.Create().SetOperationID(uuid.Nil).SetKind("resource.recheck").SetPayload(string(payload)).Save(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
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
		WithApp().
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
		WithApp().
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
	inst := Instance{
		OpenstackID: row.OpenstackID,
		Name:        row.Name,
		Status:      row.Status,
		ImageID:     row.ImageID,
		FlavorID:    row.FlavorID,
		KeyName:     row.KeyName,
		Note:        row.Note,
		Created:     row.ProviderCreatedAt,
	}
	if row.Edges.App != nil {
		inst.App = &AppSummary{
			ID:        row.Edges.App.ID.String(),
			Subdomain: firstLabel(row.Edges.App.Host),
			Host:      row.Edges.App.Host,
		}
	}
	return inst
}

func firstLabel(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	label, _, _ := strings.Cut(host, ".")
	return label
}
