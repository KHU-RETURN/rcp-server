package database

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"modernc.org/sqlite"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

type Config struct {
	Driver string // "sqlite3" | "postgres"
	DSN    string // sqlite3: "file:/var/lib/rcp/rcp.db?cache=shared&_pragma=foreign_keys(1)" | postgres: "host=localhost port=5432 user=rcp password=secret dbname=rcp sslmode=disable"
}

func NewEntClient(cfg Config) (*ent.Client, error) {
	client, err := OpenEntClient(cfg)
	if err != nil {
		return nil, err
	}

	if err := client.Schema.Create(context.Background()); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to run schema migration: %w", err)
	}

	return client, nil
}

func OpenEntClient(cfg Config) (*ent.Client, error) {
	client, err := ent.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return client, nil
}
