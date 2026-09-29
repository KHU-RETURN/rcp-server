package functions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const MaxDataValueBytes = 16 << 10
const MaxDataItemsPerFunction = 256
const MaxDataListItems = 100

var ErrInvalidData = errors.New("invalid function data collection, key, or JSON value")
var ErrDataLimit = errors.New("function data item limit reached")
var ErrDataUnavailable = errors.New("function data store unavailable")
var ErrDataNotFound = errors.New("function data item not found")

var validCollection = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var validDataKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type DataItem struct {
	Collection string          `json:"collection"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// DataStore is a separate SQLite file; every statement includes owner and function IDs.
type DataStore struct {
	db  *sql.DB
	dir string
}

func OpenDataStore(dir string) (*DataStore, error) {
	// SQLite file URIs require an absolute path. Keep metadata and bound databases
	// rooted in the same directory even when configuration uses a relative path.
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	// #nosec G703 -- dir is fixed at startup from trusted deployment configuration.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// #nosec G703 -- the directory is supplied by trusted deployment configuration.
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrInvalidData
	}
	path, err := filepath.Abs(filepath.Join(dir, "data.sqlite"))
	if err != nil {
		return nil, err
	}
	// #nosec G304 G703 -- path is a fixed basename under the validated private data directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, err
		}
	} else if errors.Is(err, os.ErrExist) {
		// #nosec G703 -- path is a fixed basename under the validated private data directory.
		info, err = os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, ErrInvalidData
		}
	} else {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	const schema = `CREATE TABLE IF NOT EXISTS function_data (
		owner_id TEXT NOT NULL,
		function_id TEXT NOT NULL,
		collection TEXT NOT NULL,
		item_key TEXT NOT NULL,
		value BLOB NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (owner_id, function_id, collection, item_key)
	) WITHOUT ROWID;
	CREATE TABLE IF NOT EXISTS deleted_functions (
		owner_id TEXT NOT NULL,
		function_id TEXT NOT NULL,
		PRIMARY KEY (owner_id, function_id)
	) WITHOUT ROWID;
	CREATE TABLE IF NOT EXISTS app_databases (
		id TEXT PRIMARY KEY,
		owner_id TEXT NOT NULL,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(owner_id, name)
	);
	CREATE TABLE IF NOT EXISTS app_database_bindings (
		owner_id TEXT NOT NULL,
		function_id TEXT NOT NULL,
		alias TEXT NOT NULL,
		database_id TEXT NOT NULL,
		PRIMARY KEY(owner_id, function_id, alias)
	) WITHOUT ROWID`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DataStore{db: db, dir: dir}, nil
}

func (s *DataStore) Close() error { return s.db.Close() }

func (s *DataStore) Get(ctx context.Context, owner, function uuid.UUID, collection, key string) (*DataItem, error) {
	if !validCollection.MatchString(collection) || !validDataKey.MatchString(key) {
		return nil, ErrInvalidData
	}
	var value []byte
	var stamp string
	err := s.db.QueryRowContext(ctx, `SELECT value, updated_at FROM function_data WHERE owner_id=? AND function_id=? AND collection=? AND item_key=?`, owner.String(), function.String(), collection, key).Scan(&value, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDataNotFound
	}
	if err != nil {
		return nil, err
	}
	updated, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return nil, err
	}
	return &DataItem{Collection: collection, Key: key, Value: value, UpdatedAt: updated}, nil
}

func (s *DataStore) List(ctx context.Context, owner, function uuid.UUID, collection string, offset int) ([]DataItem, error) {
	if !validCollection.MatchString(collection) || offset < 0 {
		return nil, ErrInvalidData
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item_key, value, updated_at FROM function_data WHERE owner_id=? AND function_id=? AND collection=? ORDER BY item_key LIMIT ? OFFSET ?`, owner.String(), function.String(), collection, MaxDataListItems, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]DataItem, 0)
	for rows.Next() {
		var item DataItem
		var stamp string
		if err := rows.Scan(&item.Key, &item.Value, &stamp); err != nil {
			return nil, err
		}
		item.Collection = collection
		item.UpdatedAt, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *DataStore) Put(ctx context.Context, owner, function uuid.UUID, collection, key string, value json.RawMessage) (*DataItem, error) {
	if !validCollection.MatchString(collection) || !validDataKey.MatchString(key) || len(value) == 0 || len(value) > MaxDataValueBytes || !json.Valid(value) {
		return nil, ErrInvalidData
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var deleted bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deleted_functions WHERE owner_id=? AND function_id=?)`, owner.String(), function.String()).Scan(&deleted)
	if err != nil {
		return nil, err
	}
	if deleted {
		return nil, ErrDataNotFound
	}
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM function_data WHERE owner_id=? AND function_id=?`, owner.String(), function.String()).Scan(&count)
	if err != nil {
		return nil, err
	}
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM function_data WHERE owner_id=? AND function_id=? AND collection=? AND item_key=?)`, owner.String(), function.String(), collection, key).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists && count >= MaxDataItemsPerFunction {
		return nil, ErrDataLimit
	}
	updated := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO function_data (owner_id,function_id,collection,item_key,value,updated_at) VALUES (?,?,?,?,?,?) ON CONFLICT(owner_id,function_id,collection,item_key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, owner.String(), function.String(), collection, key, []byte(value), updated.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DataItem{Collection: collection, Key: key, Value: value, UpdatedAt: updated}, nil
}

func (s *DataStore) Delete(ctx context.Context, owner, function uuid.UUID, collection, key string) error {
	if !validCollection.MatchString(collection) || !validDataKey.MatchString(key) {
		return ErrInvalidData
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM function_data WHERE owner_id=? AND function_id=? AND collection=? AND item_key=?`, owner.String(), function.String(), collection, key)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrDataNotFound
	}
	return nil
}

func (s *DataStore) DeleteFunction(ctx context.Context, owner, function uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM function_data WHERE owner_id=? AND function_id=?`, owner.String(), function.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_database_bindings WHERE owner_id=? AND function_id=?`, owner.String(), function.String()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO deleted_functions (owner_id,function_id) VALUES (?,?)`, owner.String(), function.String()); err != nil {
		return err
	}
	return tx.Commit()
}
