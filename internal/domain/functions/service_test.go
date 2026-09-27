package functions

import (
	"context"
	_ "embed"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

//go:embed testdata/echo.wasm
var echoWasm []byte

type memoryRepo struct {
	owner uuid.UUID
	item  *Function
}

type fakeBuilder struct{ wasm []byte }

func (b fakeBuilder) Build(_ context.Context, _ string, _ []byte) ([]byte, error) { return b.wasm, nil }

func TestSourceDeployment(t *testing.T) {
	ctx := context.Background()
	repo := &memoryRepo{}
	svc, err := NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	if _, err := svc.CreateSource(ctx, owner, "test", "rust", []byte("fn main() {}")); !errors.Is(err, ErrBuildUnavailable) {
		t.Fatalf("expected unavailable builder, got %v", err)
	}
	svc.SetBuilder(fakeBuilder{wasm: echoWasm})
	fn, err := svc.CreateSource(ctx, owner, "test", "rust", []byte("fn main() {}"))
	if err != nil {
		t.Fatal(err)
	}
	if fn.Language != "rust" || string(fn.Source) != "fn main() {}" {
		t.Fatalf("source metadata lost: %+v", fn)
	}
	if _, err := svc.UpdateSource(ctx, owner, fn.ID, "python", []byte("print('hi')")); err != nil {
		t.Fatal(err)
	}
	if repo.item.Language != "python" {
		t.Fatalf("language not updated: %s", repo.item.Language)
	}
	if _, err := svc.Update(ctx, owner, fn.ID, echoWasm); err != nil {
		t.Fatal(err)
	}
	if repo.item.Language != "wasm" || len(repo.item.Source) != 0 {
		t.Fatal("WASM replacement retained source")
	}
}

func (r *memoryRepo) List(_ context.Context, owner uuid.UUID) ([]Function, error) {
	if r.item != nil && owner == r.owner {
		return []Function{*r.item}, nil
	}
	return []Function{}, nil
}
func (r *memoryRepo) Get(_ context.Context, owner, id uuid.UUID) (*Function, error) {
	if r.item == nil || owner != r.owner || id != r.item.ID {
		return nil, ErrNotFound
	}
	return r.item, nil
}
func (r *memoryRepo) GetPublic(_ context.Context, id uuid.UUID) (*Function, error) {
	if r.item == nil || r.item.ID != id {
		return nil, ErrNotFound
	}
	return r.item, nil
}
func (r *memoryRepo) SetKey(_ context.Context, owner, id uuid.UUID, hash []byte, created, expires time.Time) (*Function, error) {
	if r.item == nil || r.owner != owner || r.item.ID != id {
		return nil, ErrNotFound
	}
	r.item.KeyHash = append([]byte(nil), hash...)
	r.item.KeyEnabled = true
	r.item.KeyCreatedAt, r.item.KeyExpiresAt = &created, &expires
	return r.item, nil
}
func (r *memoryRepo) DeleteKey(_ context.Context, owner, id uuid.UUID) (*Function, error) {
	if r.item == nil || r.owner != owner || r.item.ID != id {
		return nil, ErrNotFound
	}
	r.item.KeyHash, r.item.KeyEnabled, r.item.KeyCreatedAt, r.item.KeyExpiresAt = nil, false, nil, nil
	return r.item, nil
}
func (r *memoryRepo) Create(_ context.Context, owner uuid.UUID, name string, wasm []byte) (*Function, error) {
	if r.item != nil {
		return nil, ErrConflict
	}
	r.owner = owner
	r.item = &Function{ID: uuid.New(), OwnerID: owner, Name: name, Wasm: wasm, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return r.item, nil
}
func (r *memoryRepo) CreateDeployment(ctx context.Context, owner uuid.UUID, name string, d Deployment) (*Function, error) {
	fn, err := r.Create(ctx, owner, name, d.Wasm)
	if err == nil {
		fn.Language, fn.Source, fn.DataMode = d.Language, d.Source, d.DataMode
	}
	return fn, err
}
func (r *memoryRepo) Update(_ context.Context, owner, id uuid.UUID, wasm []byte) (*Function, error) {
	if r.item == nil || owner != r.owner || id != r.item.ID {
		return nil, ErrNotFound
	}
	r.item.Wasm = wasm
	return r.item, nil
}
func (r *memoryRepo) UpdateDeployment(ctx context.Context, owner, id uuid.UUID, d Deployment) (*Function, error) {
	fn, err := r.Update(ctx, owner, id, d.Wasm)
	if err == nil {
		fn.Language, fn.Source, fn.DataMode = d.Language, d.Source, d.DataMode
	}
	return fn, err
}
func (r *memoryRepo) Delete(_ context.Context, owner, id uuid.UUID) error {
	if r.item == nil || owner != r.owner || id != r.item.ID {
		return ErrNotFound
	}
	r.item = nil
	return nil
}

func TestDeployAndInvokeWASI(t *testing.T) {
	ctx := context.Background()
	repo := &memoryRepo{}
	svc, err := NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	fn, err := svc.Create(ctx, owner, "echo", echoWasm)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Invoke(ctx, owner, fn.ID, []byte(`{"message":"hello wasm"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "{\"MESSAGE\":\"HELLO WASM\"}" || result.Stderr != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := svc.Invoke(ctx, uuid.New(), fn.ID, []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner should not invoke function: %v", err)
	}
}

func TestRejectInvalidModuleAndInput(t *testing.T) {
	ctx := context.Background()
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	if _, err := svc.Create(ctx, owner, "Invalid Name", echoWasm); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected invalid name: %v", err)
	}
	if _, err := svc.Create(ctx, owner, "valid", []byte("not wasm")); !errors.Is(err, ErrInvalidWasm) {
		t.Fatalf("expected invalid WASM: %v", err)
	}
	fn, err := svc.Create(ctx, owner, "valid", echoWasm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invoke(ctx, owner, fn.ID, []byte(strings.Repeat("x", MaxInputBytes+1))); err == nil {
		t.Fatal("expected oversized input rejection")
	}
	if _, err := svc.Invoke(ctx, owner, fn.ID, []byte("hello")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected JSON object input rejection, got %v", err)
	}
	if _, err := svc.Invoke(ctx, owner, fn.ID, []byte(`{"ok":true}`)); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected JSON object output rejection, got %v", err)
	}
}

func TestInvocationTimeout(t *testing.T) {
	// Minimal wasm command with an endless loop in _start.
	loop := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x0a, 0x01, 0x06, '_', 's', 't', 'a', 'r', 't', 0x00, 0x00,
		0x0a, 0x09, 0x01, 0x07, 0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x0b,
	}
	ctx := context.Background()
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	fn, err := svc.Create(ctx, owner, "loop", loop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invoke(ctx, owner, fn.ID, []byte(`{}`)); !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected execution timeout, got %v", err)
	}
}

func TestRejectStartWithParameters(t *testing.T) {
	badStart := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x60, 0x01, 0x7f, 0x00,
		0x03, 0x02, 0x01, 0x00,
		0x07, 0x0a, 0x01, 0x06, '_', 's', 't', 'a', 'r', 't', 0x00, 0x00,
		0x0a, 0x04, 0x01, 0x02, 0x00, 0x0b,
	}
	ctx := context.Background()
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	if _, err := svc.Create(ctx, uuid.New(), "bad-start", badStart); !errors.Is(err, ErrInvalidWasm) {
		t.Fatalf("expected invalid WASI command, got %v", err)
	}
}

type quotaRepo struct {
	mu    sync.Mutex
	count int
}

func (r *quotaRepo) List(context.Context, uuid.UUID) ([]Function, error) {
	r.mu.Lock()
	count := r.count
	r.mu.Unlock()
	time.Sleep(15 * time.Millisecond)
	return make([]Function, count), nil
}
func (r *quotaRepo) Get(context.Context, uuid.UUID, uuid.UUID) (*Function, error) {
	return nil, ErrNotFound
}
func (r *quotaRepo) GetPublic(context.Context, uuid.UUID) (*Function, error) { return nil, ErrNotFound }
func (r *quotaRepo) SetKey(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time, time.Time) (*Function, error) {
	return nil, ErrNotFound
}
func (r *quotaRepo) DeleteKey(context.Context, uuid.UUID, uuid.UUID) (*Function, error) {
	return nil, ErrNotFound
}
func (r *quotaRepo) Create(_ context.Context, _ uuid.UUID, name string, wasm []byte) (*Function, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	return &Function{ID: uuid.New(), Name: name, Wasm: wasm}, nil
}
func (r *quotaRepo) CreateDeployment(ctx context.Context, owner uuid.UUID, name string, d Deployment) (*Function, error) {
	return r.Create(ctx, owner, name, d.Wasm)
}
func (r *quotaRepo) Update(context.Context, uuid.UUID, uuid.UUID, []byte) (*Function, error) {
	return nil, ErrNotFound
}
func (r *quotaRepo) UpdateDeployment(ctx context.Context, owner, id uuid.UUID, d Deployment) (*Function, error) {
	return r.Update(ctx, owner, id, d.Wasm)
}
func (r *quotaRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error { return ErrNotFound }

func TestConcurrentCreateRespectsQuota(t *testing.T) {
	ctx := context.Background()
	repo := &quotaRepo{count: MaxFunctionsPerUser - 1}
	svc, err := NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := svc.Create(ctx, owner, "one-more", echoWasm)
			if err != nil && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrBusy) {
				t.Errorf("unexpected create error: %v", err)
			}
		})
	}
	wg.Wait()
	if repo.count != MaxFunctionsPerUser {
		t.Fatalf("quota exceeded: %d", repo.count)
	}
}
