package database

import (
	"context"
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
)

// NewEntClient은 기존 *sql.DB를 Ent 클라이언트로 감쌉니다.
// modernc.org/sqlite는 "sqlite"로 등록되어 있으나, Ent SQL 문법 생성에는 "sqlite3"를 사용합니다.
func NewEntClient(db *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.SQLite, db)
	return ent.NewClient(ent.Driver(drv))
}

// RunMigration은 Ent 자동 마이그레이션을 실행합니다.
func RunMigration(ctx context.Context, client *ent.Client) error {
	return client.Schema.Create(ctx)
}
