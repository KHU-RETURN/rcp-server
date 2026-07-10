package storage

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrContainerNotFound        = errors.New("container not found")
	ErrContainerNotEmpty        = errors.New("container not empty, use force=true to delete all")
	ErrContainerAlreadyExists   = errors.New("container name already in use")
	ErrObjectPrefixNotFound     = errors.New("object prefix not found")
	ErrStorageOperationFailed   = errors.New("storage operation failed")
	ErrUserStorageLimitExceeded = errors.New("user storage limit exceeded")
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
	CountByOwner(ctx context.Context, ownerID uuid.UUID) (int, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Container, error)
	Delete(ctx context.Context, ownerID uuid.UUID, name string) (bool, error)
}

type Service struct {
	client     storageClient
	repo       containerRepo
	userLimits UserStorageLimits
}

func NewService(client storageClient, repo containerRepo, limits ...UserStorageLimits) *Service {
	var l UserStorageLimits
	if len(limits) > 0 {
		l = limits[0]
	}
	return &Service{client: client, repo: repo, userLimits: l}
}

func (s *Service) CreateContainer(ctx context.Context, ownerID uuid.UUID, name string) (*ContainerResponse, error) {
	name = strings.TrimSpace(name)

	// 한도 체크를 중복 이름 체크보다 먼저: 한도 초과 시 429, 이름 중복 시 409 순서 보장.
	if err := s.ensureUserStorageLimits(ctx, ownerID); err != nil {
		return nil, err
	}

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

	// Swift를 먼저 삭제: 동시 요청의 경우 client 레벨에서 404→success 처리됨.
	// Swift 성공 후 DB 삭제: 0 rows → 다른 요청이 이미 DB를 삭제 → NotFound 반환.
	// (DB-first 순서는 Swift 실패 시 DB row가 사라져 컨테이너가 영구 고아가 되는 문제 있음)
	if err := s.client.DeleteContainer(c.OpenstackName.String()); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	deleted, err := s.repo.Delete(ctx, ownerID, name)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	if !deleted {
		return ErrContainerNotFound
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
	// 클라이언트가 보내는 Content-Length는 신뢰할 수 없으므로(누락 시 -1, 위조 가능)
	// 사전 검증에 쓰지 않는다. 대신 업로드가 끝난 뒤 Swift에 실제로 기록된
	// 오브젝트 크기 기준으로 한도를 검증하고, 초과 시 방금 올린 오브젝트를 되돌린다.
	if err := s.ensureUserStorageSizeLimit(ctx, ownerID); err != nil {
		if delErr := s.client.DeleteObject(c.OpenstackName.String(), objectName); delErr != nil {
			return fmt.Errorf("%w: quota exceeded and rollback failed: %v", ErrStorageOperationFailed, delErr)
		}
		return err
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

// currentStorageUsageBytes는 유저 소유의 모든 컨테이너에 대해 오브젝트 목록을
// 병렬로 조회해 실제 사용량(바이트)을 집계한다.
func (s *Service) currentStorageUsageBytes(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	containers, err := s.repo.ListByOwner(ctx, ownerID)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}

	type listResult struct {
		bytes int64
		err   error
	}
	results := make([]listResult, len(containers))
	var wg sync.WaitGroup
	for i, c := range containers {
		wg.Add(1)
		go func(i int, swiftName string) {
			defer wg.Done()
			objs, err := s.client.ListObjects(swiftName)
			if err != nil {
				results[i].err = err
				return
			}
			for _, o := range objs {
				results[i].bytes += o.SizeBytes
			}
		}(i, c.OpenstackName.String())
	}
	wg.Wait()

	var totalBytes int64
	for _, r := range results {
		if r.err != nil {
			return 0, fmt.Errorf("%w: %v", ErrStorageOperationFailed, r.err)
		}
		totalBytes += r.bytes
	}
	return totalBytes, nil
}

// ensureUserStorageSizeLimit은 유저의 현재 실사용량이 한도를 넘는지 검증한다.
// 정확히 한도에 도달하는 것도 거부한다(>=): 한도는 "이 값보다 적게" 허용하는
// 상한으로 취급한다.
func (s *Service) ensureUserStorageSizeLimit(ctx context.Context, ownerID uuid.UUID) error {
	if s.userLimits.StorageGB <= 0 {
		return nil
	}
	totalBytes, err := s.currentStorageUsageBytes(ctx, ownerID)
	if err != nil {
		return err
	}
	limitBytes := int64(s.userLimits.StorageGB) * 1024 * 1024 * 1024
	if totalBytes >= limitBytes {
		return ErrUserStorageLimitExceeded
	}
	return nil
}

// RemainingStorageBytes는 유저가 추가로 올릴 수 있는 바이트 수를 반환한다.
// limited가 false면 한도가 설정되지 않은 것이므로 remaining 값은 의미가 없다.
func (s *Service) RemainingStorageBytes(ctx context.Context, ownerID uuid.UUID) (remaining int64, limited bool, err error) {
	if s.userLimits.StorageGB <= 0 {
		return 0, false, nil
	}
	used, err := s.currentStorageUsageBytes(ctx, ownerID)
	if err != nil {
		return 0, true, err
	}
	limitBytes := int64(s.userLimits.StorageGB) * 1024 * 1024 * 1024
	remaining = limitBytes - used
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true, nil
}

func (s *Service) ensureUserStorageLimits(ctx context.Context, ownerID uuid.UUID) error {
	if s.userLimits.Containers <= 0 {
		return nil
	}
	count, err := s.repo.CountByOwner(ctx, ownerID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageOperationFailed, err)
	}
	if count >= s.userLimits.Containers {
		return ErrUserStorageLimitExceeded
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
