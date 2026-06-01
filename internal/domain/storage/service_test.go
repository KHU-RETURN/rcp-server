package storage

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
)

var (
	testOwnerID       = uuid.New()
	testContainerUUID = uuid.New()
)

type fakeStorageClient struct {
	createContainerFn   func(name string) error
	deleteContainerFn   func(name string) error
	listObjectsFn       func(containerName string) ([]ObjectInfo, error)
	uploadObjectFn      func(containerName, objectName string, r io.Reader, contentType string) error
	downloadObjectFn    func(containerName, objectName string, w io.Writer) error
	deleteObjectFn      func(containerName, objectName string) error
	bulkDeleteObjectsFn func(containerName string, names []string) error
}

func (f *fakeStorageClient) CreateContainer(name string) error {
	if f.createContainerFn != nil {
		return f.createContainerFn(name)
	}
	return nil
}

func (f *fakeStorageClient) DeleteContainer(name string) error {
	if f.deleteContainerFn != nil {
		return f.deleteContainerFn(name)
	}
	return nil
}

func (f *fakeStorageClient) ListObjects(containerName string) ([]ObjectInfo, error) {
	if f.listObjectsFn != nil {
		return f.listObjectsFn(containerName)
	}
	return nil, nil
}

func (f *fakeStorageClient) UploadObject(containerName, objectName string, r io.Reader, contentType string) error {
	if f.uploadObjectFn != nil {
		return f.uploadObjectFn(containerName, objectName, r, contentType)
	}
	return nil
}

func (f *fakeStorageClient) DownloadObject(containerName, objectName string, w io.Writer) error {
	if f.downloadObjectFn != nil {
		return f.downloadObjectFn(containerName, objectName, w)
	}
	return nil
}

func (f *fakeStorageClient) DeleteObject(containerName, objectName string) error {
	if f.deleteObjectFn != nil {
		return f.deleteObjectFn(containerName, objectName)
	}
	return nil
}

func (f *fakeStorageClient) BulkDeleteObjects(containerName string, names []string) error {
	if f.bulkDeleteObjectsFn != nil {
		return f.bulkDeleteObjectsFn(containerName, names)
	}
	return nil
}

type fakeContainerRepo struct {
	saveFn          func(ctx context.Context, ownerID uuid.UUID, c *Container) error
	findByNameFn    func(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error)
	countByOwnerFn  func(ctx context.Context, ownerID uuid.UUID) (int, error)
	listByOwnerFn   func(ctx context.Context, ownerID uuid.UUID) ([]Container, error)
	deleteFn        func(ctx context.Context, ownerID uuid.UUID, name string) (bool, error)
}

func (r *fakeContainerRepo) Save(ctx context.Context, ownerID uuid.UUID, c *Container) error {
	if r.saveFn != nil {
		return r.saveFn(ctx, ownerID, c)
	}
	return nil
}

func (r *fakeContainerRepo) FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error) {
	if r.findByNameFn != nil {
		return r.findByNameFn(ctx, ownerID, name)
	}
	return nil, nil
}

func (r *fakeContainerRepo) CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error) {
	if r.countByOwnerFn != nil {
		return r.countByOwnerFn(ctx, ownerID)
	}
	return 0, nil
}

func (r *fakeContainerRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Container, error) {
	if r.listByOwnerFn != nil {
		return r.listByOwnerFn(ctx, ownerID)
	}
	return nil, nil
}

func (r *fakeContainerRepo) Delete(ctx context.Context, ownerID uuid.UUID, name string) (bool, error) {
	if r.deleteFn != nil {
		return r.deleteFn(ctx, ownerID, name)
	}
	return true, nil
}

func TestServiceCreateContainer(t *testing.T) {
	ctx := context.Background()

	t.Run("creates container in Swift and saves to DB", func(t *testing.T) {
		var swiftName string
		client := &fakeStorageClient{
			createContainerFn: func(name string) error {
				swiftName = name
				return nil
			},
		}
		var saved *Container
		repo := &fakeContainerRepo{
			saveFn: func(_ context.Context, _ uuid.UUID, c *Container) error {
				saved = c
				return nil
			},
		}

		svc := NewService(client, repo)
		res, err := svc.CreateContainer(ctx, testOwnerID, "my-bucket")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "my-bucket" {
			t.Fatalf("expected name my-bucket, got %q", res.Name)
		}
		if saved == nil {
			t.Fatal("expected Save to be called")
		}
		if saved.Name != "my-bucket" {
			t.Fatalf("saved name mismatch: %q", saved.Name)
		}
		if swiftName != saved.OpenstackName.String() {
			t.Fatalf("Swift name %q does not match saved OpenstackName %q", swiftName, saved.OpenstackName)
		}
	})

	t.Run("returns ErrContainerAlreadyExists when name taken", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		svc := NewService(&fakeStorageClient{}, repo)
		_, err := svc.CreateContainer(ctx, testOwnerID, "my-bucket")
		if !errors.Is(err, ErrContainerAlreadyExists) {
			t.Fatalf("expected ErrContainerAlreadyExists, got %v", err)
		}
	})

	t.Run("allows creation when under container limit", func(t *testing.T) {
		repo := &fakeContainerRepo{
			countByOwnerFn: func(_ context.Context, _ uuid.UUID) (int, error) {
				return 2, nil // 현재 2개, 한도 5개
			},
		}

		svc := NewService(&fakeStorageClient{}, repo, UserStorageLimits{Containers: 5})
		_, err := svc.CreateContainer(ctx, testOwnerID, "new-bucket")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects creation when at container limit", func(t *testing.T) {
		repo := &fakeContainerRepo{
			countByOwnerFn: func(_ context.Context, _ uuid.UUID) (int, error) {
				return 5, nil // 이미 한도 도달
			},
		}

		svc := NewService(&fakeStorageClient{}, repo, UserStorageLimits{Containers: 5})
		_, err := svc.CreateContainer(ctx, testOwnerID, "one-more")
		if !errors.Is(err, ErrUserStorageLimitExceeded) {
			t.Fatalf("expected ErrUserStorageLimitExceeded, got %v", err)
		}
	})

	t.Run("skips limit check when Containers is 0 (unlimited)", func(t *testing.T) {
		var countCalled bool
		repo := &fakeContainerRepo{
			countByOwnerFn: func(_ context.Context, _ uuid.UUID) (int, error) {
				countCalled = true
				return 999, nil
			},
		}

		svc := NewService(&fakeStorageClient{}, repo, UserStorageLimits{Containers: 0})
		_, err := svc.CreateContainer(ctx, testOwnerID, "bucket")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if countCalled {
			t.Fatal("CountByOwner should not be called when limit is disabled")
		}
	})

	t.Run("propagates repo error from CountByOwner", func(t *testing.T) {
		repo := &fakeContainerRepo{
			countByOwnerFn: func(_ context.Context, _ uuid.UUID) (int, error) {
				return 0, errors.New("db error")
			},
		}

		svc := NewService(&fakeStorageClient{}, repo, UserStorageLimits{Containers: 5})
		_, err := svc.CreateContainer(ctx, testOwnerID, "bucket")
		if !errors.Is(err, ErrStorageOperationFailed) {
			t.Fatalf("expected ErrStorageOperationFailed, got %v", err)
		}
	})
}

func TestServiceListContainers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns containers from DB", func(t *testing.T) {
		repo := &fakeContainerRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Container, error) {
				return []Container{
					{Name: "bucket-a", OpenstackName: testContainerUUID},
					{Name: "bucket-b", OpenstackName: uuid.New()},
				}, nil
			},
		}

		svc := NewService(&fakeStorageClient{}, repo)
		res, err := svc.ListContainers(ctx, testOwnerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("expected 2 containers, got %d", len(res))
		}
		if res[0].Name != "bucket-a" {
			t.Fatalf("unexpected first container: %q", res[0].Name)
		}
	})
}

func TestServiceDeleteContainer(t *testing.T) {
	ctx := context.Background()

	existingContainer := &Container{Name: "my-bucket", OpenstackName: testContainerUUID}

	t.Run("deletes empty container immediately", func(t *testing.T) {
		var deletedFromSwift string
		client := &fakeStorageClient{
			listObjectsFn:     func(_ string) ([]ObjectInfo, error) { return nil, nil },
			deleteContainerFn: func(name string) error { deletedFromSwift = name; return nil },
		}
		var deletedFromDB string
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return existingContainer, nil
			},
			deleteFn: func(_ context.Context, _ uuid.UUID, name string) (bool, error) {
				deletedFromDB = name
				return true, nil
			},
		}

		svc := NewService(client, repo)
		if err := svc.DeleteContainer(ctx, testOwnerID, "my-bucket", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deletedFromSwift != testContainerUUID.String() {
			t.Fatalf("expected Swift delete with UUID, got %q", deletedFromSwift)
		}
		if deletedFromDB != "my-bucket" {
			t.Fatalf("expected DB delete with name, got %q", deletedFromDB)
		}
	})

	t.Run("returns ErrContainerNotEmpty when objects exist and force=false", func(t *testing.T) {
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				return []ObjectInfo{{Name: "file.txt"}}, nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return existingContainer, nil
			},
		}

		svc := NewService(client, repo)
		err := svc.DeleteContainer(ctx, testOwnerID, "my-bucket", false)
		if !errors.Is(err, ErrContainerNotEmpty) {
			t.Fatalf("expected ErrContainerNotEmpty, got %v", err)
		}
	})

	t.Run("bulk-deletes objects then container when force=true", func(t *testing.T) {
		var bulkDeletedNames []string
		var deletedContainer string
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				return []ObjectInfo{{Name: "a.txt"}, {Name: "b.txt"}}, nil
			},
			bulkDeleteObjectsFn: func(_ string, names []string) error {
				bulkDeletedNames = names
				return nil
			},
			deleteContainerFn: func(name string) error { deletedContainer = name; return nil },
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return existingContainer, nil
			},
			deleteFn: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) { return true, nil },
		}

		svc := NewService(client, repo)
		if err := svc.DeleteContainer(ctx, testOwnerID, "my-bucket", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(bulkDeletedNames) != 2 || bulkDeletedNames[0] != "a.txt" {
			t.Fatalf("unexpected bulk delete names: %v", bulkDeletedNames)
		}
		if deletedContainer != testContainerUUID.String() {
			t.Fatalf("expected Swift delete with UUID, got %q", deletedContainer)
		}
	})

	t.Run("returns ErrContainerNotFound when container does not exist", func(t *testing.T) {
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return nil, nil
			},
		}

		svc := NewService(&fakeStorageClient{}, repo)
		err := svc.DeleteContainer(ctx, testOwnerID, "missing", false)
		if !errors.Is(err, ErrContainerNotFound) {
			t.Fatalf("expected ErrContainerNotFound, got %v", err)
		}
	})

	t.Run("returns ErrContainerNotFound when DB row already gone (concurrent double-delete)", func(t *testing.T) {
		var swiftDeleteCalled bool
		client := &fakeStorageClient{
			listObjectsFn:     func(_ string) ([]ObjectInfo, error) { return nil, nil },
			deleteContainerFn: func(_ string) error { swiftDeleteCalled = true; return nil },
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return existingContainer, nil
			},
			deleteFn: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
				return false, nil // 0 rows — 다른 요청이 이미 삭제
			},
		}

		svc := NewService(client, repo)
		err := svc.DeleteContainer(ctx, testOwnerID, "my-bucket", false)
		if !errors.Is(err, ErrContainerNotFound) {
			t.Fatalf("expected ErrContainerNotFound, got %v", err)
		}
		if swiftDeleteCalled {
			t.Fatal("Swift delete should not be called when DB row was already gone")
		}
	})

	t.Run("DB is deleted before Swift (quota freed atomically before external call)", func(t *testing.T) {
		var order []string
		client := &fakeStorageClient{
			listObjectsFn:     func(_ string) ([]ObjectInfo, error) { return nil, nil },
			deleteContainerFn: func(_ string) error { order = append(order, "swift"); return nil },
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return existingContainer, nil
			},
			deleteFn: func(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
				order = append(order, "db")
				return true, nil
			},
		}

		svc := NewService(client, repo)
		if err := svc.DeleteContainer(ctx, testOwnerID, "my-bucket", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(order) != 2 || order[0] != "db" || order[1] != "swift" {
			t.Fatalf("expected [db swift], got %v", order)
		}
	})
}

func TestServiceUploadObject(t *testing.T) {
	ctx := context.Background()

	t.Run("uploads to correct Swift container UUID", func(t *testing.T) {
		var uploadedTo string
		client := &fakeStorageClient{
			uploadObjectFn: func(containerName, objectName string, r io.Reader, contentType string) error {
				uploadedTo = containerName
				return nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		svc := NewService(client, repo)
		err := svc.UploadObject(ctx, testOwnerID, "my-bucket", "hello.txt", nil, "text/plain", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if uploadedTo != testContainerUUID.String() {
			t.Fatalf("expected upload to %q, got %q", testContainerUUID, uploadedTo)
		}
	})

	t.Run("returns ErrContainerNotFound for unknown container", func(t *testing.T) {
		svc := NewService(&fakeStorageClient{}, &fakeContainerRepo{})
		err := svc.UploadObject(ctx, testOwnerID, "missing", "file.txt", nil, "", 0)
		if !errors.Is(err, ErrContainerNotFound) {
			t.Fatalf("expected ErrContainerNotFound, got %v", err)
		}
	})

	t.Run("rejects upload when total storage would exceed GB limit", func(t *testing.T) {
		const gb = int64(1024 * 1024 * 1024)
		// 유저가 현재 49 GB 사용 중, 한도 50 GB, 새 파일 2 GB → 초과
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				return []ObjectInfo{{SizeBytes: 49 * gb}}, nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Container, error) {
				return []Container{{Name: "my-bucket", OpenstackName: testContainerUUID}}, nil
			},
		}

		svc := NewService(client, repo, UserStorageLimits{StorageGB: 50})
		err := svc.UploadObject(ctx, testOwnerID, "my-bucket", "big.bin", nil, "", 2*gb)
		if !errors.Is(err, ErrUserStorageLimitExceeded) {
			t.Fatalf("expected ErrUserStorageLimitExceeded, got %v", err)
		}
	})

	t.Run("allows upload when total storage is within GB limit", func(t *testing.T) {
		const gb = int64(1024 * 1024 * 1024)
		// 현재 40 GB, 한도 50 GB, 새 파일 9 GB → 허용
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				return []ObjectInfo{{SizeBytes: 40 * gb}}, nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Container, error) {
				return []Container{{Name: "my-bucket", OpenstackName: testContainerUUID}}, nil
			},
		}

		svc := NewService(client, repo, UserStorageLimits{StorageGB: 50})
		err := svc.UploadObject(ctx, testOwnerID, "my-bucket", "ok.bin", nil, "", 9*gb)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("skips GB check when StorageGB is 0 (unlimited)", func(t *testing.T) {
		var listObjectsCalled bool
		client := &fakeStorageClient{
			listObjectsFn: func(_ string) ([]ObjectInfo, error) {
				listObjectsCalled = true
				return nil, nil
			},
		}
		repo := &fakeContainerRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*Container, error) {
				return &Container{Name: "my-bucket", OpenstackName: testContainerUUID}, nil
			},
		}

		svc := NewService(client, repo, UserStorageLimits{StorageGB: 0})
		_ = svc.UploadObject(ctx, testOwnerID, "my-bucket", "file.bin", nil, "", 999*1024*1024*1024)
		if listObjectsCalled {
			t.Fatal("ListObjects should not be called when StorageGB limit is disabled")
		}
	})
}
