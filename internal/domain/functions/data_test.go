package functions

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

//go:embed testdata/data.wasm
var dataWasm []byte

func TestFunctionDataProtocol(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDataStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetDataStore(store)
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	fn, err := svc.Create(ctx, owner, "data-example", dataWasm, true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Invoke(ctx, owner, fn.ID, []byte("{\n  \"method\": \"GET\"\n}"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !json.Valid([]byte(result.Stdout)) {
		t.Fatalf("bad function response: %+v", result)
	}
	item, err := svc.GetData(ctx, owner, fn.ID, "visits", "count")
	if err != nil || string(item.Value) != "1" {
		t.Fatalf("guest write missing: %v %+v", err, item)
	}
	if _, err := svc.GetData(ctx, uuid.New(), fn.ID, "visits", "count"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner data read: %v", err)
	}
}

func TestDataStoreIsolationAndPersistence(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data")
	store, err := OpenDataStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ownerA, ownerB := uuid.New(), uuid.New()
	functionA, functionB := uuid.New(), uuid.New()
	value := json.RawMessage(`{"private":"a"}`)
	if _, err := store.Put(ctx, ownerA, functionA, "notes", "first", value); err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]uuid.UUID{{ownerB, functionA}, {ownerA, functionB}} {
		if _, err := store.Get(ctx, scope[0], scope[1], "notes", "first"); !errors.Is(err, ErrDataNotFound) {
			t.Fatalf("cross-scope read: %v", err)
		}
		items, err := store.List(ctx, scope[0], scope[1], "notes", 0)
		if err != nil || len(items) != 0 {
			t.Fatalf("cross-scope list: %v %+v", err, items)
		}
		if err := store.Delete(ctx, scope[0], scope[1], "notes", "first"); !errors.Is(err, ErrDataNotFound) {
			t.Fatalf("cross-scope delete: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenDataStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	item, err := store.Get(ctx, ownerA, functionA, "notes", "first")
	if err != nil || string(item.Value) != string(value) {
		t.Fatalf("persistent value: %v %+v", err, item)
	}
	if err := store.DeleteFunction(ctx, ownerA, functionA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ownerA, functionA, "notes", "first"); !errors.Is(err, ErrDataNotFound) {
		t.Fatalf("function data retained after deletion: %v", err)
	}
	if _, err := store.Put(ctx, ownerA, functionA, "notes", "first", value); !errors.Is(err, ErrDataNotFound) {
		t.Fatalf("deleted function wrote data again: %v", err)
	}
	for _, path := range []string{dir, filepath.Join(dir, "data.sqlite")} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("data path is not private: %s %v", path, err)
		}
	}
}

func TestDataStoreValidation(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDataStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	owner, function := uuid.New(), uuid.New()
	if _, err := store.Put(ctx, owner, function, "../other", "key", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("invalid collection: %v", err)
	}
	if _, err := store.Put(ctx, owner, function, "notes", "../../other", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("invalid key: %v", err)
	}
	if _, err := store.Put(ctx, owner, function, "notes", "key", json.RawMessage(`not-json`)); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("invalid JSON: %v", err)
	}
	for i := range MaxDataItemsPerFunction {
		key := "key-" + strconv.Itoa(i)
		if _, err := store.Put(ctx, owner, function, "notes", key, json.RawMessage(`1`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Put(ctx, owner, function, "notes", "one-too-many", json.RawMessage(`1`)); !errors.Is(err, ErrDataLimit) {
		t.Fatalf("data quota exceeded: %v", err)
	}
	if _, err := store.Put(ctx, owner, function, "notes", "key-0", json.RawMessage(`2`)); err != nil {
		t.Fatalf("update at quota should work: %v", err)
	}
	items, err := store.List(ctx, owner, function, "notes", 0)
	if err != nil || len(items) != MaxDataListItems {
		t.Fatalf("list page size: %v %d", err, len(items))
	}
	if _, err := store.Put(ctx, owner, uuid.New(), "notes", "independent", json.RawMessage(`1`)); err != nil {
		t.Fatalf("another function inherited quota: %v", err)
	}
}
