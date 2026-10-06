package operations

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/outboxevent"
	"github.com/KHU-RETURN/rcp-server/ent/resourceoperation"
	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/google/uuid"
)

var ErrUnknown = errors.New("provider result is ambiguous; creation will not be repeated")
var ErrPermanent = errors.New("operation cannot be completed")

type Outcome struct {
	ResourceID string
	Status     string
	Complete   bool
	Apply      func(*ent.Client) error
}

type Backend interface {
	Prepare(context.Context, uuid.UUID, string, string, json.RawMessage) (string, string, error)
	Execute(context.Context, *ent.ResourceOperation) (Outcome, error)
	Reconcile(context.Context) error
}

type Service struct {
	db      *ent.Client
	backend Backend
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
}

func NewService(db *ent.Client, backend Backend) *Service { return &Service{db: db, backend: backend} }

func (s *Service) Submit(ctx context.Context, owner uuid.UUID, kind, resource string, payload json.RawMessage, key string) (*api.OperationAccepted, error) {
	key = strings.TrimSpace(key)
	if len(key) > 128 {
		return nil, api.ErrOperationInvalid
	}
	if key == "" {
		key = uuid.NewString()
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(kind+"\x00"+resource+"\x00"+string(payload))))
	lookup := func() (*ent.ResourceOperation, error) {
		return s.db.ResourceOperation.Query().Where(resourceoperation.OwnerID(owner), resourceoperation.RequestKey(key)).Only(ctx)
	}
	if old, err := lookup(); err == nil {
		if old.Fingerprint != fingerprint {
			return nil, api.ErrOperationConflict
		}
		return accepted(old), nil
	} else if !ent.IsNotFound(err) {
		return nil, err
	}
	resource, active, err := s.backend.Prepare(ctx, owner, kind, resource, payload)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	op, err := tx.ResourceOperation.Create().SetOwnerID(owner).SetRequestKey(key).SetFingerprint(fingerprint).SetKind(kind).SetResourceID(resource).SetPayload(string(payload)).SetActiveKey(active).Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			if old, e := lookup(); e == nil && old.Fingerprint == fingerprint {
				return accepted(old), nil
			}
			return nil, api.ErrOperationConflict
		}
		return nil, err
	}
	if _, err = tx.OutboxEvent.Create().SetOperationID(op.ID).SetKind("execute").Save(ctx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return accepted(op), nil
}
func accepted(op *ent.ResourceOperation) *api.OperationAccepted {
	return &api.OperationAccepted{OperationID: op.ID, Status: op.Status}
}

func (s *Service) Start(parent context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		// Reconcile existing resources on startup and every minute.
		nextScan := time.Time{}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(nextScan) {
					scanCtx, stop := context.WithTimeout(ctx, 45*time.Second)
					if err := s.backend.Reconcile(scanCtx); err != nil {
						log.Printf("resource reconciliation failed: %v", err)
					}
					stop()
					nextScan = time.Now().Add(time.Minute)
				}
				for i := 0; i < 10; i++ {
					did, err := s.ProcessOne(ctx)
					if err != nil {
						log.Printf("outbox processing failed: %v", err)
						break
					}
					if !did {
						break
					}
				}
			}
		}
	}()
}
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Claim with a database compare-and-swap. A crashed worker's lease expires; token fencing
// prevents an old worker from committing after another worker has acquired its event.
func (s *Service) ProcessOne(parent context.Context) (did bool, retErr error) {
	now := time.Now()
	event, err := s.db.OutboxEvent.Query().Where(outboxevent.ProcessedAtIsNil(), outboxevent.NextAttemptAtLTE(now), outboxevent.LeaseUntilLTE(now)).Order(ent.Asc(outboxevent.FieldCreatedAt)).First(parent)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	token := uuid.NewString()
	n, err := s.db.OutboxEvent.Update().Where(outboxevent.ID(event.ID), outboxevent.ProcessedAtIsNil(), outboxevent.LeaseUntilLTE(now)).SetLeaseToken(token).SetLeaseUntil(now.Add(2 * time.Minute)).AddAttempts(1).Save(parent)
	if err != nil || n == 0 {
		return n > 0, err
	}
	defer func() {
		if retErr != nil {
			recovery, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if e := s.failClaim(recovery, event, token, retErr); e != nil {
				log.Printf("outbox failure recording failed: %v", e)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if event.Kind == "observation.changed" {
		_, err = s.db.OutboxEvent.Update().Where(outboxevent.ID(event.ID), outboxevent.LeaseToken(token), outboxevent.ProcessedAtIsNil()).SetProcessedAt(time.Now()).SetLeaseToken("").SetLeaseUntil(time.Unix(0, 0)).Save(parent)
		return true, err
	}
	if event.Kind == "resource.recheck" {
		if err = s.backend.Reconcile(ctx); err != nil {
			return true, err
		}
		_, err = s.db.OutboxEvent.Update().Where(outboxevent.ID(event.ID), outboxevent.LeaseToken(token), outboxevent.ProcessedAtIsNil()).SetProcessedAt(time.Now()).SetLeaseToken("").SetLeaseUntil(time.Unix(0, 0)).Save(parent)
		return true, err
	}
	op, err := s.db.ResourceOperation.Get(ctx, event.OperationID)
	if err != nil {
		return true, err
	}
	var outcome Outcome
	if event.Kind == "execute" {
		if !op.Dispatched {
			// The backend persists the dispatch marker immediately before a create.
			// A failed discovery query must not consume the first create attempt.
			if _, err = s.db.ResourceOperation.UpdateOneID(op.ID).SetStatus("RUNNING").Save(ctx); err != nil {
				return true, err
			}
		}
		outcome, err = s.backend.Execute(ctx, op)
	} else {
		err = s.backend.Reconcile(ctx)
		outcome = Outcome{Complete: true, Status: op.Status, ResourceID: op.ResourceID}
	}
	tx, txErr := s.db.Tx(parent)
	if txErr != nil {
		return true, txErr
	}
	defer func() { _ = tx.Rollback() }()
	// Fence in the same transaction as resource changes, operation state and the next event.
	n, txErr = tx.OutboxEvent.Update().Where(outboxevent.ID(event.ID), outboxevent.LeaseToken(token), outboxevent.ProcessedAtIsNil()).SetLeaseUntil(time.Unix(0, 0)).SetLeaseToken("").Save(parent)
	if txErr != nil {
		return true, txErr
	}
	if n == 0 {
		return true, nil
	}
	update := tx.ResourceOperation.UpdateOneID(op.ID)
	if err != nil {
		status := "VERIFYING"
		if errors.Is(err, ErrUnknown) {
			status = "UNKNOWN"
		}
		terminal := errors.Is(err, ErrPermanent) || event.Attempts+1 >= 20
		if terminal {
			status = "FAILED"
			if !op.Dispatched || strings.HasSuffix(op.Kind, ".delete") {
				update.ClearActiveKey()
			}
		}
		if event.Kind == "execute" {
			if _, txErr = update.SetStatus(status).SetLastError(err.Error()).Save(parent); txErr != nil {
				return true, txErr
			}
		}
		retry := tx.OutboxEvent.UpdateOneID(event.ID).SetLastError(err.Error())
		if terminal {
			retry.SetProcessedAt(time.Now())
		} else {
			retry.SetNextAttemptAt(time.Now().Add(backoff(event.Attempts + 1)))
		}
		if _, txErr = retry.Save(parent); txErr != nil {
			return true, txErr
		}
	} else {
		if outcome.Apply != nil {
			if txErr = outcome.Apply(tx.Client()); txErr != nil {
				return true, txErr
			}
		}
		if outcome.ResourceID != "" {
			update.SetResourceID(outcome.ResourceID)
			if !outcome.Complete {
				update.SetActiveKey(strings.Split(op.Kind, ".")[0] + ":" + outcome.ResourceID)
			}
		}
		update.SetStatus(outcome.Status).SetLastError("")
		if outcome.Complete {
			update.ClearActiveKey()
		}
		if _, txErr = update.Save(parent); txErr != nil {
			return true, txErr
		}
		ev := tx.OutboxEvent.UpdateOneID(event.ID).SetLastError("")
		if outcome.Complete {
			ev.SetProcessedAt(time.Now())
		} else {
			ev.SetNextAttemptAt(time.Now().Add(5 * time.Second))
		}
		if _, txErr = ev.Save(parent); txErr != nil {
			return true, txErr
		}
		if outcome.Complete && event.Kind == "execute" {
			payload, _ := json.Marshal(map[string]string{"operation_kind": op.Kind, "resource_id": outcome.ResourceID, "status": outcome.Status})
			if _, txErr = tx.OutboxEvent.Create().SetOperationID(op.ID).SetKind("resource.changed").SetPayload(string(payload)).Save(parent); txErr != nil {
				return true, txErr
			}
		}
	}
	return true, tx.Commit()
}
func backoff(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}

func (s *Service) failClaim(ctx context.Context, event *ent.OutboxEvent, token string, cause error) error {
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	u := tx.OutboxEvent.Update().Where(outboxevent.ID(event.ID), outboxevent.LeaseToken(token), outboxevent.ProcessedAtIsNil()).SetLeaseToken("").SetLeaseUntil(time.Unix(0, 0)).SetLastError(cause.Error()).SetNextAttemptAt(time.Now().Add(backoff(event.Attempts + 1)))
	terminal := event.Attempts+1 >= 20 || errors.Is(cause, ErrPermanent)
	if terminal {
		u.SetProcessedAt(time.Now())
	}
	n, err := u.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if event.Kind == "execute" {
		u := tx.ResourceOperation.UpdateOneID(event.OperationID).SetLastError(cause.Error())
		if terminal {
			u.SetStatus("FAILED")
		} else {
			u.SetStatus("VERIFYING")
		}
		if err = u.Exec(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
