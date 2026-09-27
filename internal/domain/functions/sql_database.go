package functions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxDatabasesPerOwner = 10
	maxSQLBytes          = 4096
	maxSQLParams         = 32
	maxSQLRows           = 100
	maxSQLResultBytes    = 64 << 10
	maxDatabasePages     = 2560 // 10 MiB with SQLite's default 4 KiB pages.
)

var (
	ErrInvalidDatabase = errors.New("invalid database name, binding, or query")
	ErrDatabaseLimit   = errors.New("database limit reached")
	ErrSQLLimit        = errors.New("database query limit reached")
	validDatabaseName  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	validBindingName   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)
)

type AppDatabase struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type DatabaseBinding struct {
	Alias        string    `json:"alias"`
	DatabaseID   uuid.UUID `json:"database_id"`
	DatabaseName string    `json:"database_name"`
}

type SQLResult struct {
	Columns      []string         `json:"columns"`
	Rows         []map[string]any `json:"rows"`
	RowsAffected int64            `json:"rows_affected"`
}

func (s *DataStore) databasePath(id uuid.UUID) string {
	return filepath.Join(s.dir, "databases", id.String()+".sqlite")
}

func (s *DataStore) openDatabase(id uuid.UUID) (*sql.DB, error) {
	path := s.databasePath(id)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrDataUnavailable
	}
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=rw&_pragma=busy_timeout(1000)&_pragma=trusted_schema(0)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", maxDatabasePages)); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *DataStore) CreateDatabase(ctx context.Context, owner uuid.UUID, name string) (*AppDatabase, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !validDatabaseName.MatchString(name) {
		return nil, ErrInvalidDatabase
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM app_databases WHERE owner_id=?`, owner.String()).Scan(&count); err != nil {
		return nil, err
	}
	if count >= maxDatabasesPerOwner {
		return nil, ErrDatabaseLimit
	}
	id := uuid.New()
	dir := filepath.Join(s.dir, "databases")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrDataUnavailable
	}
	path := s.databasePath(id)
	// #nosec G304 -- path uses a server-generated UUID under the validated private data directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(path)
		}
	}()
	db, err := s.openDatabase(id)
	if err != nil {
		return nil, err
	}
	_ = db.Close()
	created := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO app_databases(id,owner_id,name,created_at) VALUES(?,?,?,?)`, id.String(), owner.String(), name, created.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted == 0 {
		return nil, ErrConflict
	}
	remove = false
	return &AppDatabase{ID: id, Name: name, CreatedAt: created}, nil
}

func (s *DataStore) ListDatabases(ctx context.Context, owner uuid.UUID) ([]AppDatabase, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at FROM app_databases WHERE owner_id=? ORDER BY name`, owner.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]AppDatabase, 0)
	for rows.Next() {
		var rawID, stamp string
		var item AppDatabase
		if err := rows.Scan(&rawID, &item.Name, &stamp); err != nil {
			return nil, err
		}
		item.ID, err = uuid.Parse(rawID)
		if err != nil {
			return nil, err
		}
		item.CreatedAt, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *DataStore) databaseOwned(ctx context.Context, owner, id uuid.UUID) error {
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM app_databases WHERE owner_id=? AND id=?`, owner.String(), id.String()).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *DataStore) DeleteDatabase(ctx context.Context, owner, id uuid.UUID) error {
	if err := s.databaseOwned(ctx, owner, id); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_database_bindings WHERE owner_id=? AND database_id=?`, owner.String(), id.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_databases WHERE owner_id=? AND id=?`, owner.String(), id.String()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return os.Remove(s.databasePath(id))
}

func (s *DataStore) BindDatabase(ctx context.Context, owner, function, database uuid.UUID, alias string) error {
	if !validBindingName.MatchString(alias) {
		return ErrInvalidDatabase
	}
	if err := s.databaseOwned(ctx, owner, database); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_database_bindings(owner_id,function_id,alias,database_id) VALUES(?,?,?,?) ON CONFLICT(owner_id,function_id,alias) DO UPDATE SET database_id=excluded.database_id`, owner.String(), function.String(), alias, database.String())
	return err
}

func (s *DataStore) UnbindDatabase(ctx context.Context, owner, function uuid.UUID, alias string) error {
	if !validBindingName.MatchString(alias) {
		return ErrInvalidDatabase
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM app_database_bindings WHERE owner_id=? AND function_id=? AND alias=?`, owner.String(), function.String(), alias)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *DataStore) ListBindings(ctx context.Context, owner, function uuid.UUID) ([]DatabaseBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.alias,b.database_id,d.name FROM app_database_bindings b JOIN app_databases d ON d.id=b.database_id AND d.owner_id=b.owner_id WHERE b.owner_id=? AND b.function_id=? ORDER BY b.alias`, owner.String(), function.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]DatabaseBinding, 0)
	for rows.Next() {
		var item DatabaseBinding
		var rawID string
		if err := rows.Scan(&item.Alias, &rawID, &item.DatabaseName); err != nil {
			return nil, err
		}
		item.DatabaseID, err = uuid.Parse(rawID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *DataStore) QueryBinding(ctx context.Context, owner, function uuid.UUID, alias, statement string, params []any) (*SQLResult, error) {
	if !validBindingName.MatchString(alias) {
		return nil, ErrInvalidDatabase
	}
	var rawID string
	err := s.db.QueryRowContext(ctx, `SELECT b.database_id FROM app_database_bindings b JOIN app_databases d ON d.id=b.database_id AND d.owner_id=b.owner_id WHERE b.owner_id=? AND b.function_id=? AND b.alias=?`, owner.String(), function.String(), alias).Scan(&rawID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, err
	}
	return s.QueryDatabase(ctx, owner, id, statement, params)
}

func (s *DataStore) QueryDatabase(ctx context.Context, owner, id uuid.UUID, statement string, params []any) (*SQLResult, error) {
	if err := s.databaseOwned(ctx, owner, id); err != nil {
		return nil, err
	}
	read, err := validateSQL(statement, params)
	if err != nil {
		return nil, err
	}
	db, err := s.openDatabase(id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if !read {
		result, err := db.ExecContext(ctx, statement, params...)
		if err != nil {
			return nil, sqlQueryError(ctx, err)
		}
		count, err := result.RowsAffected()
		return &SQLResult{Columns: []string{}, Rows: []map[string]any{}, RowsAffected: count}, err
	}
	rows, err := db.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, sqlQueryError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &SQLResult{Columns: columns, Rows: make([]map[string]any, 0)}
	for rows.Next() {
		if len(result.Rows) >= maxSQLRows {
			return nil, ErrSQLLimit
		}
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, name := range columns {
			row[name] = values[i]
		}
		result.Rows = append(result.Rows, row)
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > maxSQLResultBytes {
			return nil, ErrSQLLimit
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sqlQueryError(ctx, err)
	}
	return result, nil
}

func sqlQueryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrTimeout
	}
	return fmt.Errorf("%w: %v", ErrInvalidDatabase, err)
}

// validateSQL accepts one statement and blocks SQLite's file and extension escape routes.
// The scanner understands quoted strings, identifiers, and comments so tokens inside them
// cannot change the statement boundary or the leading operation.
func validateSQL(statement string, params []any) (bool, error) {
	if len(statement) == 0 || len(statement) > maxSQLBytes || len(params) > maxSQLParams {
		return false, ErrInvalidDatabase
	}
	for _, value := range params {
		switch value := value.(type) {
		case nil, bool, int, int64, float64, json.Number:
		case string:
			if len(value) > 16<<10 {
				return false, ErrInvalidDatabase
			}
		default:
			return false, ErrInvalidDatabase
		}
	}
	tokens := make([]string, 0, 8)
	terminated := false
	for i := 0; i < len(statement); {
		c := statement[i]
		switch {
		case c == ' ' || c == '\n' || c == '\r' || c == '\t':
			i++
		case i+1 < len(statement) && c == '-' && statement[i+1] == '-':
			i += 2
			for i < len(statement) && statement[i] != '\n' {
				i++
			}
		case i+1 < len(statement) && c == '/' && statement[i+1] == '*':
			end := strings.Index(statement[i+2:], "*/")
			if end < 0 {
				return false, ErrInvalidDatabase
			}
			i += end + 4
		case c == '\'' || c == '"' || c == '`' || c == '[':
			close := c
			if c == '[' {
				close = ']'
			}
			i++
			found := false
			for i < len(statement) {
				if statement[i] == close {
					i++
					if i < len(statement) && statement[i] == close && c != '[' {
						i++
						continue
					}
					found = true
					break
				}
				i++
			}
			if !found || terminated {
				return false, ErrInvalidDatabase
			}
		case c == ';':
			if terminated || len(tokens) == 0 {
				return false, ErrInvalidDatabase
			}
			terminated = true
			i++
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_':
			start := i
			for i < len(statement) && ((statement[i] >= 'A' && statement[i] <= 'Z') || (statement[i] >= 'a' && statement[i] <= 'z') || (statement[i] >= '0' && statement[i] <= '9') || statement[i] == '_') {
				i++
			}
			if terminated {
				return false, ErrInvalidDatabase
			}
			token := strings.ToUpper(statement[start:i])
			switch token {
			case "ATTACH", "DETACH", "PRAGMA", "VACUUM", "LOAD_EXTENSION", "REINDEX", "ANALYZE":
				return false, ErrInvalidDatabase
			}
			tokens = append(tokens, token)
		default:
			if terminated {
				return false, ErrInvalidDatabase
			}
			i++
		}
	}
	if len(tokens) == 0 {
		return false, ErrInvalidDatabase
	}
	switch tokens[0] {
	case "SELECT", "WITH":
		return true, nil
	case "INSERT", "UPDATE", "DELETE":
		return slices.Contains(tokens, "RETURNING"), nil
	case "CREATE", "DROP":
		if len(tokens) > 1 && (tokens[1] == "TABLE" || tokens[1] == "INDEX") {
			return false, nil
		}
	case "ALTER":
		if len(tokens) > 1 && tokens[1] == "TABLE" {
			return false, nil
		}
	}
	return false, ErrInvalidDatabase
}
