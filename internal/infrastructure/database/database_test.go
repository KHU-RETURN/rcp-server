package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenEntClientDoesNotRunSchemaMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rcp.db")
	c, err := OpenEntClient(Config{
		Driver: "sqlite3",
		DSN:    "file:" + dbPath + "?cache=shared&_pragma=foreign_keys(1)",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.User.Query().Count(context.Background()); err == nil {
		t.Fatal("expected query to fail because schema was not created")
	}
}
