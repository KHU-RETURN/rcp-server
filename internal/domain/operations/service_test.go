package operations

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/outboxevent"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
	"github.com/google/uuid"
)

type fakeBackend struct {
	reconcileErr error

	calls   int
	execute func(*ent.ResourceOperation) (Outcome, error)
}

func (f *fakeBackend) Prepare(_ context.Context, _ uuid.UUID, kind, id string, _ json.RawMessage) (string, string, error) {
	if id == "" {
		id = uuid.NewString()
	}
	return id, kind + ":" + id, nil
}
func (f *fakeBackend) Execute(_ context.Context, op *ent.ResourceOperation) (Outcome, error) {
	f.calls++
	if f.execute != nil {
		return f.execute(op)
	}
	return Outcome{ResourceID: op.ResourceID, Status: "SUCCEEDED", Complete: true}, nil
}
func (f *fakeBackend) Reconcile(context.Context) error { return f.reconcileErr }
func testDB(t *testing.T) *ent.Client {
	t.Helper()
	db, err := database.NewEntClient(database.Config{Driver: "sqlite3", DSN: "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
func TestSubmitAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	s := NewService(db, &fakeBackend{})
	owner := uuid.New()
	payload := json.RawMessage(`{"name":"test"}`)
	a, err := s.Submit(ctx, owner, "instance.create", "", payload, "key")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Submit(ctx, owner, "instance.create", "", payload, "key")
	if err != nil || a.OperationID != b.OperationID {
		t.Fatalf("duplicate: %+v %v", b, err)
	}
	if _, err = s.Submit(ctx, owner, "instance.create", "", json.RawMessage(`{"name":"different"}`), "key"); !errors.Is(err, api.ErrOperationConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	if db.ResourceOperation.Query().CountX(ctx) != 1 || db.OutboxEvent.Query().CountX(ctx) != 1 {
		t.Fatal("operation and event must be committed once")
	}
	// Inject an outbox insertion failure: its operation must roll back too.
	db.OutboxEvent.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			return nil, errors.New("outbox unavailable")
		})
	})
	if _, err = s.Submit(ctx, owner, "instance.create", "", payload, "new"); err == nil {
		t.Fatal("expected failure")
	}
	if db.ResourceOperation.Query().CountX(ctx) != 1 {
		t.Fatal("operation committed without its event")
	}
}
func TestWorkerResumesAndQueuesFollowupAtomically(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	backend := &fakeBackend{}
	s := NewService(db, backend)
	a, err := s.Submit(ctx, uuid.New(), "instance.create", "", json.RawMessage(`{}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(db, backend)
	if _, err = restarted.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	op := db.ResourceOperation.GetX(ctx, a.OperationID)
	if op.Status != "SUCCEEDED" || op.ActiveKey != nil {
		t.Fatalf("operation: %+v", op)
	}
	if db.OutboxEvent.Query().Where(outboxevent.Kind("resource.changed"), outboxevent.ProcessedAtIsNil()).CountX(ctx) != 1 {
		t.Fatal("followup missing")
	}
	if _, err = restarted.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if did, err := restarted.ProcessOne(ctx); err != nil || did {
		t.Fatalf("completed work repeated: %v %v", did, err)
	}
	if backend.calls != 1 {
		t.Fatal("effect repeated")
	}
}
func TestUnknownCreationRetainsDispatchAndRetriesDiscovery(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	backend := &fakeBackend{execute: func(op *ent.ResourceOperation) (Outcome, error) {
		if !op.Dispatched {
			t.Fatal("dispatch marker lost")
		}
		return Outcome{}, ErrUnknown
	}}
	s := NewService(db, backend)
	a, err := s.Submit(ctx, uuid.New(), "instance.create", "", json.RawMessage(`{}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	db.ResourceOperation.UpdateOneID(a.OperationID).SetDispatched(true).ExecX(ctx)
	if _, err = s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	op := db.ResourceOperation.GetX(ctx, a.OperationID)
	if op.Status != "UNKNOWN" || !op.Dispatched || op.ActiveKey == nil {
		t.Fatalf("unsafe ambiguity handling: %+v", op)
	}
	event := db.OutboxEvent.Query().OnlyX(ctx)
	if event.ProcessedAt != nil || !event.NextAttemptAt.After(time.Now()) {
		t.Fatal("retry was not scheduled")
	}
}
func TestExpiredLeaseRecoveredAndLiveLeaseSkipped(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	s := NewService(db, &fakeBackend{})
	_, err := s.Submit(ctx, uuid.New(), "instance.delete", "vm", json.RawMessage(`{}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	ev := db.OutboxEvent.Query().OnlyX(ctx)
	db.OutboxEvent.UpdateOneID(ev.ID).SetLeaseUntil(time.Now().Add(time.Minute)).ExecX(ctx)
	if did, err := s.ProcessOne(ctx); did || err != nil {
		t.Fatalf("live lease processed: %v %v", did, err)
	}
	db.OutboxEvent.UpdateOneID(ev.ID).SetLeaseUntil(time.Now().Add(-time.Minute)).ExecX(ctx)
	if did, err := s.ProcessOne(ctx); !did || err != nil {
		t.Fatalf("expired lease not recovered: %v %v", did, err)
	}
}
func TestResourceWriteFailureRollsBackCompletion(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	backend := &fakeBackend{execute: func(op *ent.ResourceOperation) (Outcome, error) {
		return Outcome{ResourceID: op.ResourceID, Status: "SUCCEEDED", Complete: true, Apply: func(db *ent.Client) error { return errors.New("metadata write failed") }}, nil
	}}
	s := NewService(db, backend)
	a, err := s.Submit(ctx, uuid.New(), "instance.create", "", json.RawMessage(`{}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessOne(ctx); err == nil {
		t.Fatal("expected write failure")
	}
	op := db.ResourceOperation.GetX(ctx, a.OperationID)
	if op.Status == "SUCCEEDED" {
		t.Fatal("completion committed without resource")
	}
	if db.OutboxEvent.Query().Where(outboxevent.Kind("resource.changed")).CountX(ctx) != 0 {
		t.Fatal("completion event committed without resource")
	}
	ev := db.OutboxEvent.Query().OnlyX(ctx)
	if ev.ProcessedAt != nil || ev.LeaseToken != "" {
		t.Fatal("failed work cannot be retried")
	}
}
func TestNotificationFailureDoesNotChangeSucceededOperation(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	s := NewService(db, &fakeBackend{})
	a, err := s.Submit(ctx, uuid.New(), "instance.delete", "vm", json.RawMessage(`{}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	// A provider outage during the followup must not revert operation completion.
	s.backend = &fakeBackend{reconcileErr: errors.New("provider unavailable")}
	if _, err = s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if db.ResourceOperation.GetX(ctx, a.OperationID).Status != "SUCCEEDED" {
		t.Fatal("followup failure reverted operation")
	}
}
