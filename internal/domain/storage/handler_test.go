package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
)

var testUser = &auth.User{ID: testOwnerID}

func withTestUser(rg *gin.RouterGroup) {
	rg.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, testUser)
		c.Next()
	})
}

func newTestHandler(client *fakeStorageClient, repo *fakeContainerRepo) *Handler {
	return NewHandler(NewService(client, repo))
}

func TestHandlerListContainers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 401 when not authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/storage/containers", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		newTestHandler(&fakeStorageClient{}, &fakeContainerRepo{}).InitRoutes(r.Group(api.BasePath))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns 200 with container list", func(t *testing.T) {
		repo := &fakeContainerRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Container, error) {
				return []Container{{Name: "bucket-a"}}, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/storage/containers", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var res []ContainerResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(res) != 1 || res[0].Name != "bucket-a" {
			t.Fatalf("unexpected response: %+v", res)
		}
	})
}

func TestHandlerCreateContainer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 201 on success", func(t *testing.T) {
		body, _ := json.Marshal(CreateContainerRequest{Name: "new-bucket"})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/storage/containers", bytes.NewReader(body))
		req.Header.Set(api.HeaderContentType, api.ContentTypeJSON)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, &fakeContainerRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("returns 409 when container already exists", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "existing"}, nil
			},
		}

		body, _ := json.Marshal(CreateContainerRequest{Name: "existing"})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/storage/containers", bytes.NewReader(body))
		req.Header.Set(api.HeaderContentType, api.ContentTypeJSON)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})
}

func TestHandlerDeleteContainer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 204 on success", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
			deleteFn: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) { return true, nil },
		}

		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/storage/containers/my-bucket", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})

	t.Run("returns 409 when container not empty", func(t *testing.T) {
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				return []ObjectInfo{{Name: "file.txt"}}, nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/storage/containers/my-bucket", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(client, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("returns 404 for unknown container", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/storage/containers/missing", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, &fakeContainerRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

func TestHandlerUploadObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	makeMultipartRequest := func(t *testing.T, path, filename, content string) *http.Request {
		t.Helper()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		fw, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := io.WriteString(fw, content); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("writer.Close: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.Header.Set(api.HeaderContentType, writer.FormDataContentType())
		return req
	}

	t.Run("returns 201 with object key", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := makeMultipartRequest(t, api.BasePath+"/storage/containers/my-bucket/objects/hello.txt", "hello.txt", "hello")
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var res UploadObjectResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if res.Key != "hello.txt" {
			t.Fatalf("expected key hello.txt, got %q", res.Key)
		}
	})

	t.Run("preserves nested object key", func(t *testing.T) {
		var gotObjectName string
		client := &fakeStorageClient{
			uploadObjectFn: func(_, objectName string, _ io.Reader, _ string) error {
				gotObjectName = objectName
				return nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := makeMultipartRequest(t, api.BasePath+"/storage/containers/my-bucket/objects/dir/sub/a.txt", "a.txt", "hello")
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(client, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if gotObjectName != "dir/sub/a.txt" {
			t.Fatalf("expected object name dir/sub/a.txt, got %q", gotObjectName)
		}
		var res UploadObjectResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if res.Key != "dir/sub/a.txt" {
			t.Fatalf("expected key dir/sub/a.txt, got %q", res.Key)
		}
	})

	t.Run("returns 400 when file missing", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/storage/containers/my-bucket/objects/hello.txt", nil)
		req.Header.Set(api.HeaderContentType, "multipart/form-data")
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandlerDownloadObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("streams object content", func(t *testing.T) {
		client := &fakeStorageClient{
			downloadObjectFn: func(_, _ string, w io.Writer) error {
				_, _ = w.Write([]byte("file content"))
				return nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/storage/containers/my-bucket/objects/hello.txt", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(client, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if w.Body.String() != "file content" {
			t.Fatalf("unexpected body: %q", w.Body.String())
		}
	})
}

func TestHandlerDeleteObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 204 on success", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/storage/containers/my-bucket/objects/hello.txt", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newTestHandler(&fakeStorageClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})
}
