package storage

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrContainerNotFound      = errors.New("container not found")
	ErrContainerNotEmpty      = errors.New("container not empty, use force=true to delete all")
	ErrContainerAlreadyExists = errors.New("container name already in use")
	ErrObjectPrefixNotFound   = errors.New("object prefix not found")
	ErrStorageOperationFailed = errors.New("storage operation failed")
)

type storageClient interface {
	CreateContainer(name string) error
	DeleteContainer(name string) error
	ListObjects(containerName string) ([]ObjectInfo, error)
	UploadObject(containerName, objectName string, r io.Reader, contentType string) error
	DownloadObject(containerName, objectName string, w io.Writer) error
	DeleteObject(containerName, objectName string) error
	BulkDeleteObjects(containerName string, names []string) error
}

type containerRepo interface {
	Save(ctx context.Context, ownerID uuid.UUID, c *Container) error
	FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Container, error)
	Delete(ctx context.Context, ownerID uuid.UUID, name string) error
}

type Service struct {
	client storageClient
	repo   containerRepo
}

func NewService(client storageClient, repo containerRepo) *Service {
	return &Service{client: client, repo: repo}
}

func (s *Service) CreateContainer(ctx context.Context, ownerID uuid.UUID, name string) (*ContainerResponse, error) {
	name = strings.TrimSpace(name)

	existing, err := s.repo.FindByName(ctx, ownerID, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	if existing != nil {
		return nil, ErrContainerAlreadyExists
	}

	openstackName := uuid.New()
	if err := s.client.CreateContainer(openstackName.String()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	c := &Container{OpenstackName: openstackName, Name: name}
	if err := s.repo.Save(ctx, ownerID, c); err != nil {
		if delErr := s.client.DeleteContainer(openstackName.String()); delErr != nil {
			return nil, fmt.Errorf("%w: %v (cleanup failed: %v)", ErrStorageOperationFailed, err, delErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	return &ContainerResponse{Name: c.Name, CreatedAt: c.CreatedAt}, nil
}

func (s *Service) ListContainers(ctx context.Context, ownerID uuid.UUID) ([]ContainerResponse, error) {
	containers, err := s.repo.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	result := make([]ContainerResponse, len(containers))
	for i, container := range containers {
		result[i] = ContainerResponse{Name: container.Name, CreatedAt: container.CreatedAt}
	}
	return result, nil
}

func (s *Service) DeleteContainer(ctx context.Context, ownerID uuid.UUID, name string, force bool) error {
	c, err := s.resolveContainer(ctx, ownerID, name)
	if err != nil {
		return err
	}

	objs, err := s.client.ListObjects(c.OpenstackName.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	if len(objs) > 0 {
		if !force {
			return ErrContainerNotEmpty
		}
		names := make([]string, len(objs))
		for i, o := range objs {
			names[i] = o.Name
		}
		if err := s.client.BulkDeleteObjects(c.OpenstackName.String(), names); err != nil {
			return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
		}
	}

	if err := s.client.DeleteContainer(c.OpenstackName.String()); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	if err := s.repo.Delete(ctx, ownerID, name); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) ListObjects(ctx context.Context, ownerID uuid.UUID, containerName string) ([]ObjectInfo, error) {
	c, err := s.resolveContainer(ctx, ownerID, containerName)
	if err != nil {
		return nil, err
	}
	objs, err := s.client.ListObjects(c.OpenstackName.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return objs, nil
}

func (s *Service) UploadObject(ctx context.Context, ownerID uuid.UUID, containerName, objectName string, r io.Reader, contentType string) error {
	c, err := s.resolveContainer(ctx, ownerID, containerName)
	if err != nil {
		return err
	}
	if err := s.client.UploadObject(c.OpenstackName.String(), objectName, r, contentType); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) DownloadObject(ctx context.Context, ownerID uuid.UUID, containerName, objectName string, w io.Writer) error {
	c, err := s.resolveContainer(ctx, ownerID, containerName)
	if err != nil {
		return err
	}
	if err := s.client.DownloadObject(c.OpenstackName.String(), objectName, w); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) ArchiveObjects(ctx context.Context, ownerID uuid.UUID, containerName, prefix string, w io.Writer) error {
	c, err := s.resolveContainer(ctx, ownerID, containerName)
	if err != nil {
		return err
	}

	prefix = normalizeObjectPrefix(prefix)
	objs, err := s.client.ListObjects(c.OpenstackName.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	matches := make([]ObjectInfo, 0, len(objs))
	for _, obj := range objs {
		if prefix == "" || strings.HasPrefix(obj.Name, prefix) {
			matches = append(matches, obj)
		}
	}
	if len(matches) == 0 {
		return ErrObjectPrefixNotFound
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	zw := zip.NewWriter(w)
	for _, obj := range matches {
		entryName, err := safeZipEntryName(obj.Name)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
		}
		entry, err := zw.Create(entryName)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
		}
		if err := s.client.DownloadObject(c.OpenstackName.String(), obj.Name, entry); err != nil {
			_ = zw.Close()
			return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) DeleteObject(ctx context.Context, ownerID uuid.UUID, containerName, objectName string) error {
	c, err := s.resolveContainer(ctx, ownerID, containerName)
	if err != nil {
		return err
	}
	if err := s.client.DeleteObject(c.OpenstackName.String(), objectName); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) resolveContainer(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error) {
	c, err := s.repo.FindByName(ctx, ownerID, strings.TrimSpace(name))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	if c == nil {
		return nil, ErrContainerNotFound
	}
	return c, nil
}

func normalizeObjectPrefix(prefix string) string {
	prefix = strings.TrimLeft(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func safeZipEntryName(name string) (string, error) {
	cleaned := strings.TrimLeft(name, "/")
	if cleaned == "" {
		return "", fmt.Errorf("empty object name")
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("unsafe object name %q", name)
		}
	}
	return cleaned, nil
}
