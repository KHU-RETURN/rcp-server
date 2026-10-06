package functions

import (
	"context"
	"database/sql"
	"errors"
	"github.com/KHU-RETURN/rcp-server/internal/domain/operations"
	"github.com/google/uuid"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// InspectDatabases never initializes schemas, creates missing files or deletes data.
func (s *Service) InspectDatabases(ctx context.Context) ([]operations.Observation, error) {
	if s.data == nil {
		return []operations.Observation{{Kind: "database", ResourceID: "function-data", Status: "unconfigured", Consistency: "unknown"}}, nil
	}
	data := s.data
	result := []operations.Observation{}
	base := operations.Observation{Kind: "database", ResourceID: "function-data", Status: "healthy", Consistency: "matched"}
	if err := data.db.PingContext(ctx); err != nil {
		base.Status = "unknown"
		base.Reason = "database connection failed"
	}
	if info, err := os.Lstat(filepath.Join(data.dir, "data.sqlite")); err != nil || !info.Mode().IsRegular() {
		base.Status = "unhealthy"
		base.Consistency = "missing_file"
		base.Reason = "function metadata file missing or invalid"
	}
	result = append(result, base)
	rows, err := data.db.QueryContext(ctx, "SELECT id FROM app_databases")
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return result, err
	}
	known := map[string]bool{}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return result, ErrInvalidData
		}
		known[id] = true
		item := operations.Observation{Kind: "database", ResourceID: id, Status: "healthy", Consistency: "matched"}
		path := filepath.Join(data.dir, "databases", id+".sqlite")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			item.Status = "unhealthy"
			item.Consistency = "missing_file"
			item.Reason = "registered database has no file"
		} else if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			item.Status = "unhealthy"
			item.Consistency = "invalid_file"
			item.Reason = "database file is inaccessible or has invalid permissions"
		} else {
			dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=busy_timeout(1000)"}).String()
			db, e := sql.Open("sqlite", dsn)
			if e == nil {
				var count int
				e = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&count)
				_ = db.Close()
			}
			if e != nil {
				item.Status = "unhealthy"
				item.Reason = "database cannot be read"
			}
		}
		result = append(result, item)
	}
	entries, err := os.ReadDir(filepath.Join(data.dir, "databases"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sqlite") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".sqlite")
		if _, e := uuid.Parse(id); e != nil {
			continue
		}
		if !known[id] {
			result = append(result, operations.Observation{Kind: "database", ResourceID: id, Status: "unknown", Consistency: "unregistered_file", Reason: "retain file for administrator review"})
		}
	}
	return result, nil
}
