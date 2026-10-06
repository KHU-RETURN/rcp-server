package operations

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestOperationReadsAreOwnerScopedAndHidePayload(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	s := NewService(db, &fakeBackend{})
	owner := uuid.New()
	a, err := s.Submit(ctx, owner, "instance.create", "", json.RawMessage(`{"secret":"must not leak"}`), "key")
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []bool{false, true} {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			id := owner
			if other {
				id = uuid.New()
			}
			c.Set(auth.ContextKeyUser, &auth.User{ID: id})
			c.Next()
		})
		h := &Handler{Svc: s}
		h.InitRoutes(r.Group("/api/v1"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/operations/"+a.OperationID.String(), nil))
		expected := 200
		if other {
			expected = 404
		}
		if w.Code != expected {
			t.Fatalf("owner scope: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "request_key") {
			t.Fatal("internal payload exposed")
		}
	}
}
