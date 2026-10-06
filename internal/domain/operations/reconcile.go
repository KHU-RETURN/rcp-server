package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/container"
	"github.com/KHU-RETURN/rcp-server/ent/instance"
	"github.com/KHU-RETURN/rcp-server/ent/resourceobservation"
	"github.com/KHU-RETURN/rcp-server/ent/resourceoperation"
	"github.com/KHU-RETURN/rcp-server/ent/user"
)

type Observation struct {
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	Status      string `json:"status"`
	Consistency string `json:"consistency"`
	Reason      string `json:"reason,omitempty"`
}

func (b *LiveBackend) ReconcileResources(ctx context.Context) error {
	var errs []error
	checks := []struct {
		name string
		run  func(context.Context) error
	}{{"nova", b.reconcileInstances}, {"swift", b.reconcileContainers}, {"cinder", b.reconcileVolumes}}
	for _, check := range checks {
		err := check.run(ctx)
		status, reason := "healthy", ""
		if err != nil {
			status = "unknown"
			reason = "provider query failed"
			errs = append(errs, fmt.Errorf("%s: %w", check.name, err))
		}
		if e := b.observe(ctx, Observation{Kind: "provider", ResourceID: check.name, Status: status, Consistency: "matched", Reason: reason}, nil); e != nil {
			errs = append(errs, e)
		}
	}
	if err := b.recoverCompletedCreates(ctx); err != nil {
		errs = append(errs, err)
	}
	if b.InspectDatabases != nil {
		items, err := b.InspectDatabases(ctx)
		if err != nil {
			errs = append(errs, err)
		}
		for _, item := range items {
			if e := b.observe(ctx, item, nil); e != nil {
				errs = append(errs, e)
			}
		}
	}
	// Uses the real metadata database, without migrations or filesystem creation.
	if _, err := b.db.User.Query().Limit(1).All(ctx); err != nil {
		errs = append(errs, err)
	} else if err = b.observe(ctx, Observation{Kind: "database", ResourceID: "platform", Status: "healthy", Consistency: "matched"}, nil); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Observation, safe metadata repair and notification are committed together.
func (b *LiveBackend) observe(ctx context.Context, item Observation, repair func(*ent.Client) error) error {
	tx, err := b.db.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	key := item.Kind + ":" + item.ResourceID
	old, err := tx.ResourceObservation.Query().Where(resourceobservation.ResourceKey(key)).Only(ctx)
	changed := false
	switch {
	case ent.IsNotFound(err):
		changed = true
		_, err = tx.ResourceObservation.Create().SetResourceKey(key).SetKind(item.Kind).SetResourceID(item.ResourceID).SetStatus(item.Status).SetConsistency(item.Consistency).SetReason(item.Reason).Save(ctx)
	case err == nil:
		changed = old.Status != item.Status || old.Consistency != item.Consistency || old.Reason != item.Reason
		_, err = tx.ResourceObservation.UpdateOneID(old.ID).SetStatus(item.Status).SetConsistency(item.Consistency).SetReason(item.Reason).SetObservedAt(time.Now()).Save(ctx)
	}
	if err != nil {
		return err
	}
	if repair != nil {
		if err = repair(tx.Client()); err != nil {
			return err
		}
	}
	if changed {
		payload, _ := json.Marshal(item)
		if _, err = tx.OutboxEvent.Create().SetOperationID(uuid.Nil).SetKind("observation.changed").SetPayload(string(payload)).Save(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (b *LiveBackend) reconcileInstances(ctx context.Context) error {
	sc, err := b.client(ctx, "instance")
	if err != nil {
		return err
	}
	pages, err := servers.List(sc, servers.ListOpts{}).AllPages()
	if err != nil {
		return err
	}
	all, err := servers.ExtractServers(pages)
	if err != nil {
		return err
	}
	rows, err := b.db.Instance.Query().WithOwner().All(ctx)
	if err != nil {
		return err
	}
	live := map[string]servers.Server{}
	known := map[string]bool{}
	for _, srv := range all {
		live[srv.ID] = srv
	}
	for _, row := range rows {
		known[row.OpenstackID] = true
		srv, ok := live[row.OpenstackID]
		if !ok {
			found, e := servers.Get(sc, row.OpenstackID).Extract()
			if e == nil {
				srv = *found
			} else if notFound(e) {
				if err = b.observe(ctx, Observation{"instance", row.OpenstackID, "MISSING", "missing_in_provider", "confirmed by individual lookup"}, nil); err != nil {
					return err
				}
				continue
			} else {
				return e
			}
		}
		image, flavor := resourceRef(srv.Image), resourceRef(srv.Flavor)
		item := Observation{"instance", srv.ID, srv.Status, "matched", ""}
		if err = b.observe(ctx, item, func(db *ent.Client) error {
			u := db.Instance.UpdateOneID(row.ID).SetStatus(srv.Status).SetName(srv.Name)
			if image != "" {
				u.SetImageID(image)
			}
			if flavor != "" {
				u.SetFlavorID(flavor)
			}
			return u.Exec(ctx)
		}); err != nil {
			return err
		}
	}
	for _, srv := range all {
		if known[srv.ID] {
			continue
		}
		item := Observation{"instance", srv.ID, srv.Status, "missing_in_rcp", "owner cannot be established"}
		// Only provider ownership metadata tied to an existing RCP user permits import.
		owner, e := uuid.Parse(srv.Metadata[ownerTag])
		image, flavor := resourceRef(srv.Image), resourceRef(srv.Flavor)
		var repair func(*ent.Client) error
		if e == nil && image != "" && flavor != "" {
			exists, e := b.db.User.Query().Where(user.ID(owner)).Exist(ctx)
			if e != nil {
				return e
			}
			if exists {
				item.Consistency = "matched"
				item.Reason = "recovered from provider ownership metadata"
				repair = func(db *ent.Client) error {
					exists, e := db.Instance.Query().Where(instance.OpenstackID(srv.ID)).Exist(ctx)
					if e != nil || exists {
						return e
					}
					return db.Instance.Create().SetOwnerID(owner).SetOpenstackID(srv.ID).SetName(srv.Name).SetStatus(srv.Status).SetImageID(image).SetFlavorID(flavor).SetKeyName(srv.KeyName).SetProviderCreatedAt(srv.Created).Exec(ctx)
				}
			}
		}
		if err = b.observe(ctx, item, repair); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, srv := range all {
		seen[srv.ID] = true
	}
	for _, row := range rows {
		seen[row.OpenstackID] = true
	}
	return b.observeAbsent(ctx, "instance", seen, func(id string) error { _, err := servers.Get(sc, id).Extract(); return err })
}
func resourceRef(value map[string]any) string { id, _ := value["id"].(string); return id }
func (b *LiveBackend) reconcileContainers(ctx context.Context) error {
	sc, err := b.client(ctx, "container")
	if err != nil {
		return err
	}
	pages, err := containers.List(sc, containers.ListOpts{Full: true}).AllPages()
	if err != nil {
		return err
	}
	all, err := containers.ExtractInfo(pages)
	if err != nil {
		return err
	}
	rows, err := b.db.Container.Query().All(ctx)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, row := range rows {
		name := row.OpenstackName.String()
		known[name] = true
		_, e := containers.Get(sc, name, nil).Extract()
		item := Observation{"container", name, "healthy", "matched", ""}
		if notFound(e) {
			item.Status = "MISSING"
			item.Consistency = "missing_in_provider"
			item.Reason = "confirmed by individual lookup"
		} else if e != nil {
			return e
		}
		if err = b.observe(ctx, item, nil); err != nil {
			return err
		}
	}
	for _, c := range all {
		if known[c.Name] {
			continue
		}
		// Tagged pending creates are recovered by their durable operation. Legacy
		// Swift UUIDs have no trustworthy RCP display name/owner: retain and report.

		item := Observation{"container", c.Name, "healthy", "missing_in_rcp", "awaiting operation recovery or administrator review"}
		var repair func(*ent.Client) error
		// New operations carry the trustworthy display name and owner; recover even
		// if their execution event exhausted its retry budget before Swift became visible.
		h, e := containers.Get(sc, c.Name, nil).ExtractMetadata()
		if e != nil {
			return e
		}
		operationID, e := uuid.Parse(metadataValue(h, operationTag))
		if e == nil {
			op, e := b.db.ResourceOperation.Get(ctx, operationID)
			if e == nil && op.Kind == "container.create" && op.ResourceID == c.Name && op.OwnerID.String() == metadataValue(h, ownerTag) {
				var req struct{ Name string }
				ownerExists, e := b.db.User.Query().Where(user.ID(op.OwnerID)).Exist(ctx)
				if e != nil {
					return e
				}
				if ownerExists && decode(op.Payload, &req) == nil {
					id, e := uuid.Parse(c.Name)
					if e != nil {
						return e
					}
					item.Consistency = "matched"
					item.Reason = "recovered from durable operation"
					repair = func(db *ent.Client) error {
						exists, e := db.Container.Query().Where(container.OpenstackName(id)).Exist(ctx)
						if e != nil || exists {
							return e
						}
						return db.Container.Create().SetOwnerID(op.OwnerID).SetOpenstackName(id).SetName(strings.TrimSpace(req.Name)).Exec(ctx)
					}
				}
			} else if e != nil && !ent.IsNotFound(e) {
				return e
			}
		}
		if err = b.observe(ctx, item, repair); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, c := range all {
		seen[c.Name] = true
	}
	for _, row := range rows {
		seen[row.OpenstackName.String()] = true
	}
	return b.observeAbsent(ctx, "container", seen, func(id string) error { _, err := containers.Get(sc, id, nil).Extract(); return err })
}
func (b *LiveBackend) reconcileVolumes(ctx context.Context) error {
	sc, err := b.client(ctx, "volume")
	if err != nil {
		return err
	}
	pages, err := volumes.List(sc, volumes.ListOpts{}).AllPages()
	if err != nil {
		return err
	}
	all, err := volumes.ExtractVolumes(pages)
	if err != nil {
		return err
	}
	// Cinder is already the volume inventory; this repository has no local volume table.
	for _, v := range all {
		consistency, reason := "matched", ""
		owner, e := uuid.Parse(v.Metadata[ownerTag])
		if e != nil {
			consistency = "missing_owner"
			reason = "administrator review required"
		} else {
			exists, e := b.db.User.Query().Where(user.ID(owner)).Exist(ctx)
			if e != nil {
				return e
			}
			if !exists {
				consistency = "missing_owner"
				reason = "RCP owner no longer exists"
			}
		}
		status := v.Status
		if strings.HasPrefix(status, "error") {
			reason = "provider reports volume error"
		}
		if err = b.observe(ctx, Observation{"volume", v.ID, status, consistency, reason}, nil); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, v := range all {
		seen[v.ID] = true
	}
	return b.observeAbsent(ctx, "volume", seen, func(id string) error { _, err := volumes.Get(sc, id).Extract(); return err })
}

func (b *LiveBackend) observeAbsent(ctx context.Context, kind string, seen map[string]bool, lookup func(string) error) error {
	rows, err := b.db.ResourceObservation.Query().Where(resourceobservation.Kind(kind)).All(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if seen[row.ResourceID] {
			continue
		}
		err := lookup(row.ResourceID)
		if notFound(err) {
			if e := b.observe(ctx, Observation{Kind: kind, ResourceID: row.ResourceID, Status: "MISSING", Consistency: "missing_in_provider", Reason: "confirmed by individual lookup"}, nil); e != nil {
				return e
			}
		} else if err != nil {
			return err
		} else {
			return errors.New("provider inventory changed during reconciliation; retry required")
		}
	}
	return nil
}

// A late provider completion can outlive the execute event's retry budget.
// Recover only confirmed existing resources; this scan never issues a new create.
func (b *LiveBackend) recoverCompletedCreates(ctx context.Context) error {
	ops, err := b.db.ResourceOperation.Query().Where(resourceoperation.Status("FAILED"), resourceoperation.KindIn("instance.create", "volume.create", "container.create")).Limit(100).All(ctx)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if op.Kind == "container.create" {
			sc, e := b.client(ctx, "container")
			if e != nil {
				continue
			}
			if _, e = containers.Get(sc, op.ResourceID, nil).Extract(); e != nil {
				continue
			}
		} else if !op.Dispatched {
			continue
		}
		outcome, e := b.Execute(ctx, op)
		if e != nil || !outcome.Complete {
			continue
		}
		tx, e := b.db.Tx(ctx)
		if e != nil {
			return e
		}
		n, e := tx.ResourceOperation.Update().Where(resourceoperation.ID(op.ID), resourceoperation.Status("FAILED")).SetStatus("SUCCEEDED").SetResourceID(outcome.ResourceID).SetLastError("").ClearActiveKey().Save(ctx)
		if e == nil && n > 0 && outcome.Apply != nil {
			e = outcome.Apply(tx.Client())
		}
		if e == nil && n > 0 {
			_, e = tx.OutboxEvent.Create().SetOperationID(op.ID).SetKind("resource.changed").Save(ctx)
		}
		if e != nil {
			_ = tx.Rollback()
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	return nil
}
