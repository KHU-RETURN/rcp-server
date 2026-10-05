package functions

import (
	"context"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectDatabasesReportsMissingAndOrphanWithoutCreatingFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// #nosec G302 -- owner-only directory requires execute permission.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := OpenDataStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()
	missing := uuid.NewString()
	orphan := uuid.NewString()
	if _, err = data.db.ExecContext(ctx, "INSERT INTO app_databases(id,owner_id,name,created_at) VALUES(?,?,?,?)", missing, uuid.NewString(), "missing", "now"); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(dir, "databases"), 0700); err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(dir, "databases", orphan+".sqlite")
	if err = os.WriteFile(orphanPath, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := &Service{data: data}
	items, err := svc.InspectDatabases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundMissing, foundOrphan := false, false
	for _, item := range items {
		if item.ResourceID == missing {
			foundMissing = item.Consistency == "missing_file"
		}
		if item.ResourceID == orphan {
			foundOrphan = item.Consistency == "unregistered_file"
		}
	}
	if !foundMissing || !foundOrphan {
		t.Fatalf("observations: %+v", items)
	}
	if _, err = os.Stat(filepath.Join(dir, "databases", missing+".sqlite")); !os.IsNotExist(err) {
		t.Fatal("inspection created missing database")
	}
	// #nosec G304 -- UUID-named fixture under t.TempDir.
	if raw, err := os.ReadFile(orphanPath); err != nil || string(raw) != "retain" {
		t.Fatal("inspection changed orphan data")
	}
}
