package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestDatabaseBindingCRUDIsolationAndPersistence(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data")
	store, err := OpenDataStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	owner, other := uuid.New(), uuid.New()
	function, otherFunction := uuid.New(), uuid.New()
	db, err := store.CreateDatabase(ctx, owner, "notes-db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDatabase(ctx, owner, "notes-db"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := store.QueryDatabase(ctx, other, db.ID, `SELECT 1`, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner queried database: %v", err)
	}
	if err := store.BindDatabase(ctx, other, otherFunction, db.ID, "DB"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner bound database: %v", err)
	}
	if err := store.BindDatabase(ctx, owner, function, db.ID, "DB"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryBinding(ctx, owner, otherFunction, "DB", `SELECT 1`, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unbound function queried database: %v", err)
	}
	if _, err := store.QueryBinding(ctx, owner, function, "DB", `CREATE TABLE notes (id INTEGER PRIMARY KEY, text TEXT NOT NULL)`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryBinding(ctx, owner, function, "DB", `INSERT INTO notes (text) VALUES (?)`, []any{"hello"}); err != nil {
		t.Fatal(err)
	}
	result, err := store.QueryBinding(ctx, owner, function, "DB", `SELECT id, text FROM notes WHERE text = ?`, []any{"hello"})
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["text"] != "hello" {
		t.Fatalf("select: %+v %v", result, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenDataStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err = store.QueryBinding(ctx, owner, function, "DB", `SELECT text FROM notes`, nil)
	if err != nil || len(result.Rows) != 1 {
		t.Fatalf("database not persistent: %+v %v", result, err)
	}
	if _, err := store.QueryBinding(ctx, owner, function, "DB", `UPDATE notes SET text = ? WHERE id = ?`, []any{"updated", 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryBinding(ctx, owner, function, "DB", `DELETE FROM notes WHERE id = ?`, []any{1}); err != nil {
		t.Fatal(err)
	}
	result, err = store.QueryBinding(ctx, owner, function, "DB", `SELECT text FROM notes`, nil)
	if err != nil || len(result.Rows) != 0 {
		t.Fatalf("delete failed: %+v %v", result, err)
	}
	if err := store.DeleteDatabase(ctx, other, db.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner deleted database: %v", err)
	}
	if err := store.DeleteDatabase(ctx, owner, db.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.databasePath(db.ID)); !os.IsNotExist(err) {
		t.Fatalf("database file remains: %v", err)
	}
	bindings, err := store.ListBindings(ctx, owner, function)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("binding remains: %+v %v", bindings, err)
	}
}

func TestDatabaseRejectsFileEscapeAndMultipleStatements(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDataStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	owner := uuid.New()
	db, err := store.CreateDatabase(ctx, owner, "sandbox")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`ATTACH DATABASE '/tmp/outside.sqlite' AS outside`,
		`SELECT 1; ATTACH DATABASE '/tmp/outside.sqlite' AS outside`,
		`SELECT load_extension('x')`,
		`PRAGMA writable_schema=ON`,
		`CREATE VIRTUAL TABLE x USING fts5(content)`,
	} {
		if _, err := store.QueryDatabase(ctx, owner, db.ID, statement, nil); !errors.Is(err, ErrInvalidDatabase) {
			t.Errorf("accepted %q: %v", statement, err)
		}
	}
	for _, statement := range []string{
		`SELECT ';' AS value`,
		`SELECT 1 /* ; ATTACH DATABASE 'x' */`,
		`SELECT 1; -- comment`,
	} {
		if _, err := store.QueryDatabase(ctx, owner, db.ID, statement, nil); err != nil {
			t.Errorf("rejected safe SQL %q: %v", statement, err)
		}
	}
}

func TestSQLProtocolUsesBoundDatabase(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDataStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	owner, function := uuid.New(), uuid.New()
	db, err := store.CreateDatabase(ctx, owner, "protocol")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindDatabase(ctx, owner, function, db.ID, "DB"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryDatabase(ctx, owner, db.ID, `CREATE TABLE notes (text TEXT)`, nil); err != nil {
		t.Fatal(err)
	}
	stream := &dataInput{ctx: ctx, replies: make(chan []byte, 1)}
	out := &dataOutput{ctx: ctx, store: store, owner: owner, function: function, input: stream}
	request := []byte(`{"$rcp":"sql","binding":"DB","sql":"INSERT INTO notes (text) VALUES (?)","params":["from-function"]}` + "\n")
	if _, err := out.Write(request); err != nil {
		t.Fatal(err)
	}
	var reply dataReply
	if err := json.Unmarshal(<-stream.replies, &reply); err != nil || !reply.OK || reply.Type != "sql.result" {
		t.Fatalf("protocol reply: %+v %v", reply, err)
	}
	result, err := store.QueryDatabase(ctx, owner, db.ID, `SELECT text FROM notes`, nil)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["text"] != "from-function" {
		t.Fatalf("guest write missing: %+v %v", result, err)
	}
}

func TestDatabaseHTTPBindingsAndAuthorization(t *testing.T) {
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
	fn, err := svc.Create(ctx, owner, "db-app", echoWasm, true)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		user := owner
		if c.GetHeader("X-Test-Other-User") != "" {
			user = uuid.New()
		}
		c.Set(auth.ContextKeyUser, &auth.User{ID: user})
		c.Next()
	})
	NewHandler(svc).InitRoutes(router.Group("/api/v1"))
	request := func(method, path, body string, other bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if other {
			req.Header.Set("X-Test-Other-User", "1")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	created := request(http.MethodPost, "/api/v1/databases", `{"name":"app-db"}`, false)
	if created.Code != http.StatusCreated {
		t.Fatalf("create database: %d %s", created.Code, created.Body.String())
	}
	var db AppDatabase
	if err := json.Unmarshal(created.Body.Bytes(), &db); err != nil {
		t.Fatal(err)
	}
	dbPath := "/api/v1/databases/" + db.ID.String()
	if got := request(http.MethodPost, dbPath+"/query", `{"sql":"CREATE TABLE notes (text TEXT)","params":[]}`, true); got.Code != http.StatusNotFound {
		t.Fatalf("other owner queried database: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, dbPath+"/query", `{"sql":"CREATE TABLE notes (text TEXT)","params":[]}`, false); got.Code != http.StatusOK {
		t.Fatalf("create table: %d %s", got.Code, got.Body.String())
	}
	bindings := "/api/v1/functions/" + fn.ID.String() + "/databases/DB"
	if got := request(http.MethodPut, bindings, `{"database_id":"`+db.ID.String()+`"}`, false); got.Code != http.StatusNoContent {
		t.Fatalf("bind: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/functions/"+fn.ID.String()+"/databases", "", false); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"alias":"DB"`)) {
		t.Fatalf("list binding: %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/v1/functions/"+fn.ID.String()+"/databases", "", true); got.Code != http.StatusNotFound {
		t.Fatalf("other owner listed binding: %d", got.Code)
	}
	if got := request(http.MethodPost, dbPath+"/query", `{"sql":"ATTACH DATABASE '/tmp/outside.sqlite' AS outside"}`, false); got.Code != http.StatusBadRequest {
		t.Fatalf("unsafe SQL accepted: %d %s", got.Code, got.Body.String())
	}
}
