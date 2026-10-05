package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
	osapi "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/container"
	"github.com/KHU-RETURN/rcp-server/ent/instance"
	"github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/blockstorage"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	infra "github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
)

const operationTag = "rcp_operation_id"
const ownerTag = "rcp_owner_id"

type LiveBackend struct {
	db               *ent.Client
	provider         *gophercloud.ProviderClient
	defaultNetwork   string
	InspectDatabases func(context.Context) ([]Observation, error)
}

func NewLiveBackend(db *ent.Client, p *gophercloud.ProviderClient, network string) *LiveBackend {
	return &LiveBackend{db: db, provider: p, defaultNetwork: network}
}
func (b *LiveBackend) client(ctx context.Context, kind string) (*gophercloud.ServiceClient, error) {
	if b.provider == nil {
		return nil, errors.New("OpenStack is not configured")
	}
	p := *b.provider
	p.Context = ctx
	opts := gophercloud.EndpointOpts{Region: infra.Region}
	switch kind {
	case "instance":
		return osapi.NewComputeV2(&p, opts)
	case "container":
		return osapi.NewObjectStorageV1(&p, opts)
	default:
		return osapi.NewBlockStorageV3(&p, opts)
	}
}
func notFound(err error) bool       { var e gophercloud.ErrDefault404; return errors.As(err, &e) }
func permanent(reason string) error { return fmt.Errorf("%w: %s", ErrPermanent, reason) }
func decode(raw string, dst any) error {
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return api.ErrOperationInvalid
	}
	return nil
}

func (b *LiveBackend) Prepare(ctx context.Context, owner uuid.UUID, kind, resource string, payload json.RawMessage) (string, string, error) {
	parts := strings.Split(kind, ".")
	if len(parts) != 2 || (parts[1] != "create" && parts[1] != "delete") {
		return "", "", api.ErrOperationInvalid
	}
	switch kind {
	case "instance.create":
		var opts compute.CreateServerOpts
		if decode(string(payload), &opts) != nil || strings.TrimSpace(opts.Name) == "" || opts.ImageRef == "" || opts.FlavorRef == "" {
			return "", "", api.ErrOperationInvalid
		}
		resource = uuid.NewString()
	case "container.create":
		var req struct{ Name string }
		if decode(string(payload), &req) != nil || strings.TrimSpace(req.Name) == "" {
			return "", "", api.ErrOperationInvalid
		}
		exists, err := b.db.Container.Query().Where(container.Name(strings.TrimSpace(req.Name)), container.HasOwnerWith(user.ID(owner))).Exist(ctx)
		if err != nil {
			return "", "", err
		}
		if exists {
			return "", "", api.ErrOperationConflict
		}
		return uuid.NewString(), "container-name:" + owner.String() + ":" + strings.TrimSpace(req.Name), nil
	case "volume.create":
		var req blockstorage.CreateVolumeRequest
		if decode(string(payload), &req) != nil || strings.TrimSpace(req.Name) == "" || req.SizeGiB < 1 {
			return "", "", api.ErrOperationInvalid
		}
		if req.SnapshotID != "" {
			sc, err := b.client(ctx, "volume")
			if err != nil {
				return "", "", err
			}
			snap, err := snapshots.Get(sc, req.SnapshotID).Extract()
			if err != nil {
				return "", "", err
			}
			if snap.Metadata[ownerTag] != owner.String() || snap.Status != "available" || req.SizeGiB < snap.Size {
				return "", "", api.ErrOperationInvalid
			}
		}
		resource = uuid.NewString()
	case "instance.delete":
		exists, err := b.db.Instance.Query().Where(instance.OpenstackID(resource), instance.HasOwnerWith(user.ID(owner))).Exist(ctx)
		if err != nil {
			return "", "", err
		}
		if !exists {
			return "", "", api.ErrOperationNotFound
		}
	case "container.delete":
		row, err := b.db.Container.Query().Where(container.Name(resource), container.HasOwnerWith(user.ID(owner))).Only(ctx)
		if ent.IsNotFound(err) {
			return "", "", api.ErrOperationNotFound
		}
		if err != nil {
			return "", "", err
		}
		resource = row.OpenstackName.String()
	case "volume.delete":
		sc, err := b.client(ctx, "volume")
		if err != nil {
			return "", "", err
		}
		v, err := volumes.Get(sc, resource).Extract()
		if notFound(err) {
			return "", "", api.ErrOperationNotFound
		}
		if err != nil {
			return "", "", err
		}
		if v.Metadata[ownerTag] != owner.String() {
			return "", "", api.ErrOperationNotFound
		}
		if v.Status != "available" {
			return "", "", api.ErrOperationConflict
		}
	default:
		return "", "", api.ErrOperationInvalid
	}
	return resource, parts[0] + ":" + resource, nil
}
func (b *LiveBackend) Execute(ctx context.Context, op *ent.ResourceOperation) (Outcome, error) {
	if exists, err := b.db.User.Query().Where(user.ID(op.OwnerID)).Exist(ctx); err != nil {
		return Outcome{}, err
	} else if !exists {
		return Outcome{}, permanent("owner no longer exists")
	}
	switch op.Kind {
	case "instance.create":
		return b.createInstance(ctx, op)
	case "container.create":
		return b.createContainer(ctx, op)
	case "volume.create":
		return b.createVolume(ctx, op)
	case "instance.delete", "container.delete", "volume.delete":
		return b.delete(ctx, op)
	}
	return Outcome{}, permanent("unsupported operation")
}
func (b *LiveBackend) createInstance(ctx context.Context, op *ent.ResourceOperation) (Outcome, error) {
	sc, err := b.client(ctx, "instance")
	if err != nil {
		return Outcome{}, err
	}
	var opts compute.CreateServerOpts
	if err = decode(op.Payload, &opts); err != nil {
		return Outcome{}, permanent("invalid payload")
	}
	pages, err := servers.List(sc, servers.ListOpts{}).AllPages()
	if err != nil {
		return Outcome{}, err
	}
	all, err := servers.ExtractServers(pages)
	if err != nil {
		return Outcome{}, err
	}
	var found *servers.Server
	for i := range all {
		if all[i].Metadata[operationTag] == op.ID.String() && all[i].Metadata[ownerTag] == op.OwnerID.String() {
			if found != nil {
				return Outcome{}, permanent("multiple servers match operation")
			}
			found = &all[i]
		}
	}
	if found == nil {
		if op.Dispatched {
			return Outcome{}, ErrUnknown
		}
		networks := make([]servers.Network, 0, len(opts.Networks))
		for _, n := range opts.Networks {
			networks = append(networks, servers.Network{UUID: n.UUID})
		}
		if len(networks) == 0 && b.defaultNetwork != "" {
			networks = append(networks, servers.Network{UUID: b.defaultNetwork})
		}
		if err = b.markDispatched(ctx, op); err != nil {
			return Outcome{}, err
		}
		created, err := servers.Create(sc, keypairs.CreateOptsExt{CreateOptsBuilder: servers.CreateOpts{Name: opts.Name, ImageRef: opts.ImageRef, FlavorRef: opts.FlavorRef, SecurityGroups: opts.SecurityGroups, Networks: networks, Metadata: map[string]string{operationTag: op.ID.String(), ownerTag: op.OwnerID.String()}}, KeyName: opts.KeyName}).Extract()
		if err != nil {
			return Outcome{}, err
		}
		found, err = servers.Get(sc, created.ID).Extract()
		if err != nil {
			return Outcome{}, err
		}
	}
	if found.Status == "ERROR" {
		return Outcome{}, permanent("server entered ERROR")
	}
	result := Outcome{ResourceID: found.ID, Status: "VERIFYING", Complete: found.Status == "ACTIVE"}
	if result.Complete {
		result.Status = "SUCCEEDED"
	}
	srv := *found
	result.Apply = func(db *ent.Client) error {
		row, err := db.Instance.Query().Where(instance.OpenstackID(srv.ID)).WithOwner().Only(ctx)
		if err == nil {
			if row.Edges.Owner == nil || row.Edges.Owner.ID != op.OwnerID {
				return permanent("server ownership conflict")
			}
			return db.Instance.UpdateOneID(row.ID).SetStatus(srv.Status).SetName(srv.Name).SetNote(opts.Note).SetKeyName(opts.KeyName).Exec(ctx)
		}
		if !ent.IsNotFound(err) {
			return err
		}
		return db.Instance.Create().SetOwnerID(op.OwnerID).SetOpenstackID(srv.ID).SetName(srv.Name).SetStatus(srv.Status).SetImageID(opts.ImageRef).SetFlavorID(opts.FlavorRef).SetKeyName(opts.KeyName).SetNote(opts.Note).SetProviderCreatedAt(srv.Created).Exec(ctx)
	}
	return result, nil
}
func (b *LiveBackend) createContainer(ctx context.Context, op *ent.ResourceOperation) (Outcome, error) {
	sc, err := b.client(ctx, "container")
	if err != nil {
		return Outcome{}, err
	}
	var req struct{ Name string }
	if err = decode(op.Payload, &req); err != nil {
		return Outcome{}, permanent("invalid payload")
	}
	h, err := containers.Get(sc, op.ResourceID, nil).ExtractMetadata()
	if notFound(err) {
		// Swift PUT at the preallocated UUID is idempotent.
		if err = containers.Create(sc, op.ResourceID, containers.CreateOpts{Metadata: map[string]string{operationTag: op.ID.String(), ownerTag: op.OwnerID.String()}}).Err; err != nil {
			return Outcome{}, err
		}
		h, err = containers.Get(sc, op.ResourceID, nil).ExtractMetadata()
	}
	if err != nil {
		return Outcome{}, err
	}
	if metadataValue(h, operationTag) != op.ID.String() || metadataValue(h, ownerTag) != op.OwnerID.String() {
		return Outcome{}, permanent("container ownership conflict")
	}
	id, err := uuid.Parse(op.ResourceID)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{ResourceID: op.ResourceID, Status: "SUCCEEDED", Complete: true, Apply: func(db *ent.Client) error {
		row, err := db.Container.Query().Where(container.OpenstackName(id)).WithOwner().Only(ctx)
		if err == nil {
			if row.Edges.Owner == nil || row.Edges.Owner.ID != op.OwnerID {
				return permanent("container ownership conflict")
			}
			return nil
		}
		if !ent.IsNotFound(err) {
			return err
		}
		return db.Container.Create().SetOwnerID(op.OwnerID).SetOpenstackName(id).SetName(strings.TrimSpace(req.Name)).Exec(ctx)
	}}, nil
}
func (b *LiveBackend) createVolume(ctx context.Context, op *ent.ResourceOperation) (Outcome, error) {
	sc, err := b.client(ctx, "volume")
	if err != nil {
		return Outcome{}, err
	}
	var req blockstorage.CreateVolumeRequest
	if err = decode(op.Payload, &req); err != nil {
		return Outcome{}, permanent("invalid payload")
	}
	pages, err := volumes.List(sc, volumes.ListOpts{}).AllPages()
	if err != nil {
		return Outcome{}, err
	}
	all, err := volumes.ExtractVolumes(pages)
	if err != nil {
		return Outcome{}, err
	}
	var found *volumes.Volume
	for i := range all {
		if all[i].Metadata[operationTag] == op.ID.String() && all[i].Metadata[ownerTag] == op.OwnerID.String() {
			if found != nil {
				return Outcome{}, permanent("multiple volumes match operation")
			}
			found = &all[i]
		}
	}
	if found == nil {
		if op.Dispatched {
			return Outcome{}, ErrUnknown
		}
		if err = b.markDispatched(ctx, op); err != nil {
			return Outcome{}, err
		}
		found, err = volumes.Create(sc, volumes.CreateOpts{Name: strings.TrimSpace(req.Name), Description: req.Description, Size: req.SizeGiB, VolumeType: req.VolumeType, AvailabilityZone: req.AvailabilityZone, SnapshotID: req.SnapshotID, Metadata: map[string]string{operationTag: op.ID.String(), ownerTag: op.OwnerID.String()}}).Extract()
		if err != nil {
			return Outcome{}, err
		}
	}
	if strings.HasPrefix(found.Status, "error") {
		return Outcome{}, permanent("volume entered error state")
	}
	complete := found.Status == "available" || found.Status == "in-use"
	status := "VERIFYING"
	if complete {
		status = "SUCCEEDED"
	}
	return Outcome{ResourceID: found.ID, Status: status, Complete: complete}, nil
}
func (b *LiveBackend) delete(ctx context.Context, op *ent.ResourceOperation) (Outcome, error) {
	kind := strings.Split(op.Kind, ".")[0]
	sc, err := b.client(ctx, kind)
	if err != nil {
		return Outcome{}, err
	}
	switch kind {
	case "instance":
		_, err = servers.Get(sc, op.ResourceID).Extract()
		if err == nil {
			err = servers.Delete(sc, op.ResourceID).ExtractErr()
			if err != nil && !notFound(err) {
				return Outcome{}, err
			}
			_, err = servers.Get(sc, op.ResourceID).Extract()
		}
	case "container":
		_, err = containers.Get(sc, op.ResourceID, nil).Extract()
		if err == nil {
			var req struct{ Force bool }
			if e := decode(op.Payload, &req); e != nil {
				return Outcome{}, permanent("invalid payload")
			}
			pages, e := objects.List(sc, op.ResourceID, objects.ListOpts{Full: true}).AllPages()
			if e != nil {
				return Outcome{}, e
			}
			all, e := objects.ExtractInfo(pages)
			if e != nil {
				return Outcome{}, e
			}
			if len(all) > 0 && !req.Force {
				return Outcome{}, permanent("container is not empty")
			}
			// Only the queued container UUID can be affected by a force deletion.
			for _, o := range all {
				if e = objects.Delete(sc, op.ResourceID, o.Name, nil).Err; e != nil && !notFound(e) {
					return Outcome{}, e
				}
			}
			if e = containers.Delete(sc, op.ResourceID).Err; e != nil && !notFound(e) {
				return Outcome{}, e
			}
			_, err = containers.Get(sc, op.ResourceID, nil).Extract()
		}
	case "volume":
		var v *volumes.Volume
		v, err = volumes.Get(sc, op.ResourceID).Extract()
		if err == nil {
			if v.Metadata[ownerTag] != op.OwnerID.String() {
				return Outcome{}, permanent("volume ownership changed")
			}
			if v.Status != "deleting" {
				if v.Status != "available" {
					return Outcome{}, permanent("volume is attached or busy")
				}
				if err = volumes.Delete(sc, op.ResourceID, nil).ExtractErr(); err != nil && !notFound(err) {
					return Outcome{}, err
				}
			}
			_, err = volumes.Get(sc, op.ResourceID).Extract()
		}
	}
	if err == nil {
		return Outcome{ResourceID: op.ResourceID, Status: "VERIFYING"}, nil
	}
	if !notFound(err) {
		return Outcome{}, err
	}
	result := Outcome{ResourceID: op.ResourceID, Status: "SUCCEEDED", Complete: true}
	result.Apply = func(db *ent.Client) error {
		switch kind {
		case "instance":
			_, err := db.Instance.Delete().Where(instance.OpenstackID(op.ResourceID), instance.HasOwnerWith(user.ID(op.OwnerID))).Exec(ctx)
			return err
		case "container":
			id, e := uuid.Parse(op.ResourceID)
			if e != nil {
				return e
			}
			_, err := db.Container.Delete().Where(container.OpenstackName(id), container.HasOwnerWith(user.ID(op.OwnerID))).Exec(ctx)
			return err
		}
		return nil
	}
	return result, nil
}

func metadataValue(values map[string]string, key string) string {
	for k, v := range values {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
func (b *LiveBackend) markDispatched(ctx context.Context, op *ent.ResourceOperation) error {
	_, err := b.db.ResourceOperation.UpdateOneID(op.ID).SetDispatched(true).Save(ctx)
	return err
}
