package storage

import (
	"time"

	"github.com/google/uuid"
)

// Container — DB에 저장되는 소유권 메타데이터.
type Container struct {
	OpenstackName uuid.UUID // Swift container 실제 이름 (UUID)
	Name          string    // 사용자에게 노출되는 이름
	CreatedAt     time.Time
}

// ObjectInfo — Swift에서 조회한 object 메타데이터.
type ObjectInfo struct {
	Name         string    `json:"name"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	LastModified time.Time `json:"last_modified"`
}

// StatusError — 클라이언트 레이어가 반환하며 서비스 레이어가 상태 코드로 분기할 수 있게 한다.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// --- Request/Response DTOs ---

type CreateContainerRequest struct {
	Name string `json:"name" binding:"required"`
}

type ContainerResponse struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type UploadObjectResponse struct {
	Key string `json:"key"`
}
