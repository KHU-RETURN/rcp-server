package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
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
	drv, err := entsql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// sqlite는 파일 단위 write lock이라 writer 커넥션을 1개로 제한해
	// 풀 내부 경쟁으로 인한 SQLITE_BUSY를 막는다 (읽기 전용 DSN은 제외).
	if cfg.Driver == "sqlite3" && !strings.Contains(cfg.DSN, "mode=ro") {
		drv.DB().SetMaxOpenConns(1)
	}
	return ent.NewClient(ent.Driver(drv)), nil
}
