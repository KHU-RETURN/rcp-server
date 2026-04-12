package access

import (
	"context"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/keypair"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/google/uuid"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) SaveKeyPair(ctx context.Context, ownerID uuid.UUID, kp *KeyPair) error {
	return r.client.KeyPair.Create().
		SetOwnerID(ownerID).
		SetOpenstackName(kp.Name).
		SetFingerprint(kp.Fingerprint).
		SetPublicKey(kp.PublicKey).
		SetSourceType(keypair.SourceTypeUserUploaded).
		Exec(ctx)
}

func (r *Repository) DeleteByName(ctx context.Context, ownerID uuid.UUID, name string) error {
	_, err := r.client.KeyPair.Delete().
		Where(
			keypair.OpenstackName(name),
			keypair.HasOwnerWith(entuser.ID(ownerID)),
		).
		Exec(ctx)
	return err
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]KeyPair, error) {
	kps, err := r.client.KeyPair.Query().
		Where(keypair.HasOwnerWith(entuser.ID(ownerID))).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]KeyPair, len(kps))
	for i, kp := range kps {
		result[i] = KeyPair{
			Name:        kp.OpenstackName,
			Fingerprint: kp.Fingerprint,
			PublicKey:   kp.PublicKey,
		}
	}
	return result, nil
}

func (r *Repository) FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPair, error) {
	kp, err := r.client.KeyPair.Query().
		Where(
			keypair.OpenstackName(name),
			keypair.HasOwnerWith(entuser.ID(ownerID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &KeyPair{
		Name:        kp.OpenstackName,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}
