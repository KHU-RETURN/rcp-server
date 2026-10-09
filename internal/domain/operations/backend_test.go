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
	"github.com/KHU-RETURN/rcp-server/ent/outboxevent"

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

func TestQuotaRejectionFinishesOperationWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name, kind, payload, body string
		code                      int
	}{
		{"instance count", "instance.create", `{"Name":"vm","ImageRef":"image","FlavorRef":"flavor"}`, `{"forbidden":{"message":"Quota exceeded for instances: Requested 1, but already used 10 of 10 instances","code":403}}`, 403},
		{"CPU", "instance.create", `{"Name":"vm","ImageRef":"image","FlavorRef":"flavor"}`, `{"forbidden":{"message":"Quota exceeded for cores","code":403}}`, 403},
		{"RAM", "instance.create", `{"Name":"vm","ImageRef":"image","FlavorRef":"flavor"}`, `{"overLimit":{"message":"Quota exceeded for ram","code":413}}`, 413},
		{"volume count", "volume.create", `{"name":"volume","sizeGiB":1}`, `{"overLimit":{"message":"Maximum number of volumes allowed (10) exceeded for quota 'volumes'.","code":413}}`, 413},
		{"volume capacity", "volume.create", `{"name":"volume","sizeGiB":1}`, `{"overLimit":{"message":"Quota exceeded for gigabytes","code":403}}`, 403},
		{"container count", "container.create", `{"Name":"data"}`, "Reached container limit of 10", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := testDB(t)
			owner := ownerRow(t, db)
			creates := 0
			b := liveTestBackend(t, db, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost || r.Method == http.MethodPut {
					creates++
					w.WriteHeader(tc.code)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				switch r.URL.Path {
				case "/servers/detail":
					_, _ = w.Write([]byte(`{"servers":[]}`))
				case "/volumes/detail":
					_, _ = w.Write([]byte(`{"volumes":[]}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			svc := NewService(db, b)
			accepted, err := svc.Submit(ctx, owner, tc.kind, "", json.RawMessage(tc.payload), "quota-key")
			if err != nil {
				t.Fatal(err)
			}
			if did, err := svc.ProcessOne(ctx); err != nil || !did {
				t.Fatalf("process: %v %v", did, err)
			}
			op := db.ResourceOperation.GetX(ctx, accepted.OperationID)
			if op.Status != "FAILED" || op.ActiveKey != nil || op.Dispatched || op.LastError != ErrQuotaExceeded.Error() {
				t.Fatalf("quota rejection not terminal: %+v", op)
			}
			if view(op).LastError != "resource quota exceeded" {
				t.Fatal("quota reason missing from response")
			}
			event := db.OutboxEvent.Query().Where(outboxevent.OperationID(op.ID)).OnlyX(ctx)
			if event.ProcessedAt == nil || event.Attempts != 1 {
				t.Fatalf("event: %+v", event)
			}
			if did, err := svc.ProcessOne(ctx); err != nil || did {
				t.Fatalf("rejected request retried: %v %v", did, err)
			}
			duplicate, err := svc.Submit(ctx, owner, tc.kind, "", json.RawMessage(tc.payload), "quota-key")
			if err != nil || duplicate.OperationID != op.ID || duplicate.Status != "FAILED" {
				t.Fatalf("idempotency: %+v %v", duplicate, err)
			}
			if creates != 1 || db.Instance.Query().CountX(ctx) != 0 || db.Container.Query().CountX(ctx) != 0 {
				t.Fatal("rejected create allocated a resource or was repeated")
			}
			// A new request key is required after quota has been freed. In particular,
			// the rejected Swift name must no longer be held by the old operation.
			if _, err := svc.Submit(ctx, owner, tc.kind, "", json.RawMessage(tc.payload), "new-key"); err != nil {
				t.Fatalf("reservation was not released: %v", err)
			}
		})
	}
}

func TestCreateFailureKeepsUncertainAndRateLimitErrorsRetryable(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
	}{
		{"provider failure", `{"message":"quota service unavailable"}`, 500},
		{"rate limit", `{"overLimit":{"message":"Rate limit exceeded"}}`, 413},
		{"forbidden", `{"forbidden":{"message":"Not authorized to read quota"}}`, 403},
		{"large payload", "Request Entity Too Large", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := gophercloud.ErrUnexpectedResponseCode{Actual: tc.code, Body: []byte(tc.body)}
			err := createFailure(original)
			var preserved gophercloud.ErrUnexpectedResponseCode
			if errors.Is(err, ErrPermanent) || errors.Is(err, ErrQuotaExceeded) || !errors.As(err, &preserved) || preserved.Actual != tc.code || string(preserved.Body) != tc.body {
				t.Fatalf("misclassified: %v", err)
			}
		})
	}
}
