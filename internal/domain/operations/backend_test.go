package operations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
)

func liveTestBackend(t *testing.T, db *ent.Client, handler http.HandlerFunc) *LiveBackend {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	provider := &gophercloud.ProviderClient{TokenID: "test", HTTPClient: *server.Client(), EndpointLocator: func(gophercloud.EndpointOpts) (string, error) { return server.URL + "/", nil }}
	return NewLiveBackend(db, provider, "network")
}
func ownerRow(t *testing.T, db *ent.Client) uuid.UUID {
	t.Helper()
	return db.User.Create().SetEmail(uuid.NewString() + "@test.dev").SetName("test").SetGoogleID(uuid.NewString()).SetGoogleAccessToken("").SetGoogleRefreshToken("").SetGoogleTokenExpiry(time.Now()).SaveX(context.Background()).ID
}
func TestVMRecoveryNeverRecreatesAmbiguousRequest(t *testing.T) {
	for _, found := range []bool{false, true} {
		t.Run(map[bool]string{false: "not visible", true: "visible"}[found], func(t *testing.T) {
			ctx := context.Background()
			db := testDB(t)
			owner := ownerRow(t, db)
			id := uuid.New()
			creates := 0
			b := liveTestBackend(t, db, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					creates++
					w.WriteHeader(500)
					return
				}
				list := []any{}
				if found {
					list = append(list, map[string]any{"id": "vm", "name": "test", "status": "ACTIVE", "metadata": map[string]string{operationTag: id.String(), ownerTag: owner.String()}, "image": map[string]string{"id": "image"}, "flavor": map[string]string{"id": "flavor"}})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"servers": list})
			})
			payload, _ := json.Marshal(compute.CreateServerOpts{Name: "test", ImageRef: "image", FlavorRef: "flavor", Note: "preserved"})
			op := db.ResourceOperation.Create().SetID(id).SetOwnerID(owner).SetRequestKey("k").SetFingerprint("f").SetKind("instance.create").SetResourceID(uuid.NewString()).SetPayload(string(payload)).SetDispatched(true).SaveX(ctx)
			result, err := b.Execute(ctx, op)
			if found {
				if err != nil || !result.Complete {
					t.Fatalf("recovery: %+v %v", result, err)
				}
				if err = result.Apply(db); err != nil {
					t.Fatal(err)
				}
				row := db.Instance.Query().OnlyX(ctx)
				if row.Note != "preserved" {
					t.Fatal("request metadata lost")
				}
			} else if !errors.Is(err, ErrUnknown) {
				t.Fatalf("expected ambiguity: %v", err)
			}
			if creates != 0 {
				t.Fatal("ambiguous create was repeated")
			}
		})
	}
}
func TestSwiftRecoveryUsesAllocatedUUIDAndCanonicalMetadata(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	owner := ownerRow(t, db)
	id := uuid.New()
	resource := uuid.NewString()
	puts := 0
	exists := false
	b := liveTestBackend(t, db, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+resource {
			t.Errorf("wrong target: %s", r.URL.Path)
		}
		if r.Method == "PUT" {
			puts++
			exists = true
			w.WriteHeader(201)
			return
		}
		if !exists {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("X-Container-Meta-Rcp_operation_id", id.String())
		w.Header().Set("X-Container-Meta-Rcp_owner_id", owner.String())
		w.WriteHeader(204)
	})
	op := db.ResourceOperation.Create().SetID(id).SetOwnerID(owner).SetRequestKey("k").SetFingerprint("f").SetKind("container.create").SetResourceID(resource).SetPayload(`{"name":"data"}`).SetDispatched(true).SaveX(ctx)
	for i := 0; i < 2; i++ {
		out, err := b.Execute(ctx, op)
		if err != nil {
			t.Fatal(err)
		}
		if err = out.Apply(db); err != nil {
			t.Fatal(err)
		}
	}
	if puts != 1 || db.Container.Query().CountX(ctx) != 1 {
		t.Fatalf("duplicated: PUTs=%d", puts)
	}
}
func TestDeletionOwnershipCheckedBeforeQueueing(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	owner := ownerRow(t, db)
	b := NewLiveBackend(db, nil, "")
	_, _, err := b.Prepare(ctx, owner, "instance.delete", "not-owned", json.RawMessage(`{}`))
	if !errors.Is(err, api.ErrOperationNotFound) {
		t.Fatalf("ownership: %v", err)
	}
}
