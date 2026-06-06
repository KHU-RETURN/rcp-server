package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/migrate"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
)

type fakeHealthChecker struct {
	openstackErr error
	storageErr   error
	sshErr       error
}

func (f fakeHealthChecker) CheckOpenStack(context.Context) error  { return f.openstackErr }
func (f fakeHealthChecker) CheckStorage(context.Context) error    { return f.storageErr }
func (f fakeHealthChecker) CheckSSHGateway(context.Context) error { return f.sshErr }

func newAdminTestClient(t *testing.T, name string) *ent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	if err := client.Schema.Create(context.Background(), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	return client
}

func TestAdminDashboardEndpointsRequireAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("RCP_ADMIN_EMAILS", "admin@return.dev")

	client := newAdminTestClient(t, "admin-authz")

	handler := Init(client)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, &auth.User{
			ID:    uuid.New(),
			Email: "student@return.dev",
			Name:  "Student",
		})
	})
	group.Use(auth.AdminRequired())
	handler.InitRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}
}

func TestAdminDashboardEndpointsReturnReadOnlyInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("RCP_ADMIN_EMAILS", "admin@return.dev")

	ctx := context.Background()
	client := newAdminTestClient(t, "admin-dashboard")

	adminUser := client.User.Create().
		SetEmail("admin@return.dev").
		SetName("Admin").
		SetGoogleID("google-admin").
		SetGoogleAccessToken("access").
		SetGoogleRefreshToken("refresh").
		SetGoogleTokenExpiry(time.Now()).
		SaveX(ctx)
	student := client.User.Create().
		SetEmail("student@return.dev").
		SetName("Student").
		SetGoogleID("google-student").
		SetGoogleAccessToken("access").
		SetGoogleRefreshToken("refresh").
		SetGoogleTokenExpiry(time.Now()).
		SaveX(ctx)

	instance := client.Instance.Create().
		SetOwner(student).
		SetOpenstackID("vm-001").
		SetName("student-web").
		SetStatus("ACTIVE").
		SetImageID("ubuntu").
		SetFlavorID("m1.small").
		SetProviderCreatedAt(time.Now()).
		SaveX(ctx)
	client.Container.Create().
		SetOwner(student).
		SetOpenstackName(uuid.New()).
		SetName("student-bucket").
		SaveX(ctx)
	client.KeyPair.Create().
		SetOwner(student).
		SetOpenstackName("student-key").
		SetFingerprint("fp").
		SetPublicKey("ssh-ed25519 AAAA").
		SetSourceType("user_uploaded").
		SaveX(ctx)
	client.App.Create().
		SetInstance(instance).
		SetHost("student.rcp.dev").
		SaveX(ctx)

	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, &auth.User{
			ID:    adminUser.ID,
			Email: adminUser.Email,
			Name:  adminUser.Name,
		})
	})
	group.Use(auth.AdminRequired())
	Init(client).InitRoutes(group)

	t.Run("summary counts users and visible resources", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/summary", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body SummaryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		if body.Users != 2 || body.Instances != 1 || body.Containers != 1 || body.Keypairs != 1 {
			t.Fatalf("unexpected summary: %+v", body)
		}
		if body.StatusCounts["ACTIVE"] != 1 {
			t.Fatalf("expected ACTIVE count 1, got %+v", body.StatusCounts)
		}
	})

	t.Run("users include resource counts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body PaginatedUsersResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode users: %v", err)
		}
		if body.Pagination.Page != 1 || body.Pagination.PerPage != 10 || body.Pagination.Total != 2 {
			t.Fatalf("unexpected pagination: %+v", body.Pagination)
		}
		if len(body.Items) != 2 {
			t.Fatalf("expected 2 users, got %d", len(body.Items))
		}
		var foundStudent *UserResponse
		for i := range body.Items {
			if body.Items[i].Email == "student@return.dev" {
				foundStudent = &body.Items[i]
			}
		}
		if foundStudent == nil {
			t.Fatal("student user missing")
		}
		if foundStudent.InstanceCount != 1 || foundStudent.ContainerCount != 1 || foundStudent.KeypairCount != 1 {
			t.Fatalf("unexpected resource counts: %+v", *foundStudent)
		}
	})

	t.Run("instances include owner details", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/instances?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body PaginatedInstancesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode instances: %v", err)
		}
		if len(body.Items) != 1 || body.Pagination.Total != 1 {
			t.Fatalf("unexpected instances response: %+v", body)
		}
		if body.Items[0].OwnerEmail != "student@return.dev" {
			t.Fatalf("unexpected instance response: %+v", body.Items[0])
		}
	})

	t.Run("instance detail includes owner and status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/instances/vm-001", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body InstanceResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode instance detail: %v", err)
		}
		if body.ID != "vm-001" || body.Status != "ACTIVE" || body.OwnerEmail != "student@return.dev" {
			t.Fatalf("unexpected instance detail: %+v", body)
		}
	})

	t.Run("containers include owner details", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/containers?page=1&limit=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body PaginatedContainersResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode containers: %v", err)
		}
		if len(body.Items) != 1 || body.Pagination.Total != 1 {
			t.Fatalf("unexpected containers response: %+v", body)
		}
		if body.Items[0].OwnerEmail != "student@return.dev" || body.Items[0].Status != "ready" {
			t.Fatalf("unexpected container response: %+v", body.Items[0])
		}
	})

	t.Run("container detail includes owner and status", func(t *testing.T) {
		var containerID string
		rows, err := client.Container.Query().All(ctx)
		if err != nil {
			t.Fatalf("query containers: %v", err)
		}
		for _, row := range rows {
			if row.Name == "student-bucket" {
				containerID = row.ID.String()
			}
		}
		if containerID == "" {
			t.Fatal("student container missing")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/containers/"+containerID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body ContainerResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode container detail: %v", err)
		}
		if body.ID != containerID || body.Status != "ready" || body.OwnerEmail != "student@return.dev" {
			t.Fatalf("unexpected container detail: %+v", body)
		}
	})

	t.Run("user resources include owned visible resource status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+student.ID.String()+"/resources", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var body UserResourcesResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode resources: %v", err)
		}
		if body.User.Email != "student@return.dev" {
			t.Fatalf("unexpected user: %+v", body.User)
		}
		if len(body.Instances) != 1 || body.Instances[0].Status != "ACTIVE" {
			t.Fatalf("unexpected instance resources: %+v", body.Instances)
		}
		if len(body.Containers) != 1 || body.Containers[0].Status != "ready" {
			t.Fatalf("unexpected container resources: %+v", body.Containers)
		}
		if len(body.Keypairs) != 1 || body.Keypairs[0].Status != "registered" {
			t.Fatalf("unexpected keypair resources: %+v", body.Keypairs)
		}
	})
}

func TestAdminSystemReturnsRealHealthStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := newAdminTestClient(t, "admin-system")
	router := gin.New()
	Init(client, WithHealthChecker(fakeHealthChecker{
		openstackErr: nil,
		storageErr:   errors.New("storage down"),
		sshErr:       ErrHealthCheckUnconfigured,
	})).InitRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body SystemResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode system: %v", err)
	}
	if body.APIStatus != "healthy" || body.OpenStackStatus != "healthy" || body.StorageStatus != "unhealthy" || body.SSHGatewayStatus != "unconfigured" {
		t.Fatalf("unexpected system statuses: %+v", body)
	}
	if body.Message == "" {
		t.Fatal("expected non-empty system message")
	}
}
