package functions

import (
	"context"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent"
	entfunction "github.com/KHU-RETURN/rcp-server/ent/function"
	"github.com/KHU-RETURN/rcp-server/ent/predicate"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/google/uuid"
)

type EntRepository struct{ client *ent.Client }

func NewRepository(client *ent.Client) *EntRepository { return &EntRepository{client: client} }

func owned(owner uuid.UUID) predicate.Function {
	return entfunction.HasOwnerWith(entuser.ID(owner))
}

func toFunction(row *ent.Function) Function {
	fn := Function{ID: row.ID, Name: row.Name, Wasm: row.Wasm, Language: row.Language, DataMode: row.DataMode, Source: row.Source, KeyHash: row.KeyHash, KeyEnabled: len(row.KeyHash) > 0, KeyCreatedAt: row.KeyCreatedAt, KeyExpiresAt: row.KeyExpiresAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.Edges.Owner != nil {
		fn.OwnerID = row.Edges.Owner.ID
	}
	return fn
}

func (r *EntRepository) List(ctx context.Context, owner uuid.UUID) ([]Function, error) {
	rows, err := r.client.Function.Query().Where(owned(owner)).
		Select(entfunction.FieldID, entfunction.FieldName, entfunction.FieldLanguage, entfunction.FieldDataMode, entfunction.FieldKeyHash, entfunction.FieldKeyCreatedAt, entfunction.FieldKeyExpiresAt, entfunction.FieldCreatedAt, entfunction.FieldUpdatedAt).
		Order(ent.Desc(entfunction.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Function, 0, len(rows))
	for _, row := range rows {
		result = append(result, toFunction(row))
	}
	return result, nil
}

func (r *EntRepository) Get(ctx context.Context, owner, id uuid.UUID) (*Function, error) {
	row, err := r.client.Function.Query().Where(entfunction.ID(id), owned(owner)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	fn.OwnerID = owner
	return &fn, nil
}

func (r *EntRepository) GetPublic(ctx context.Context, id uuid.UUID) (*Function, error) {
	row, err := r.client.Function.Query().Where(entfunction.ID(id)).WithOwner().Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	return &fn, nil
}

func (r *EntRepository) SetKey(ctx context.Context, owner, id uuid.UUID, hash []byte, created, expires time.Time) (*Function, error) {
	row, err := r.client.Function.UpdateOneID(id).Where(owned(owner)).SetKeyHash(hash).SetKeyCreatedAt(created).SetKeyExpiresAt(expires).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	return &fn, nil
}

func (r *EntRepository) DeleteKey(ctx context.Context, owner, id uuid.UUID) (*Function, error) {
	row, err := r.client.Function.UpdateOneID(id).Where(owned(owner)).ClearKeyHash().ClearKeyCreatedAt().ClearKeyExpiresAt().Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	return &fn, nil
}

func (r *EntRepository) Create(ctx context.Context, owner uuid.UUID, name string, wasm []byte) (*Function, error) {
	return r.CreateDeployment(ctx, owner, name, Deployment{Language: "wasm", Wasm: wasm})
}

func (r *EntRepository) CreateDeployment(ctx context.Context, owner uuid.UUID, name string, deployment Deployment) (*Function, error) {
	row, err := r.client.Function.Create().SetOwnerID(owner).SetName(name).SetWasm(deployment.Wasm).SetLanguage(deployment.Language).SetDataMode(deployment.DataMode).SetSource(deployment.Source).Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	return &fn, nil
}

func (r *EntRepository) Update(ctx context.Context, owner, id uuid.UUID, wasm []byte) (*Function, error) {
	return r.UpdateDeployment(ctx, owner, id, Deployment{Language: "wasm", Wasm: wasm})
}

func (r *EntRepository) UpdateDeployment(ctx context.Context, owner, id uuid.UUID, deployment Deployment) (*Function, error) {
	row, err := r.client.Function.UpdateOneID(id).Where(owned(owner)).SetWasm(deployment.Wasm).SetLanguage(deployment.Language).SetDataMode(deployment.DataMode).SetSource(deployment.Source).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fn := toFunction(row)
	return &fn, nil
}

func (r *EntRepository) Delete(ctx context.Context, owner, id uuid.UUID) error {
	n, err := r.client.Function.Delete().Where(entfunction.ID(id), owned(owner)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
