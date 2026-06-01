package storage

import (
	"time"

	"github.com/google/uuid"
)

type Container struct {
	OpenstackName uuid.UUID
	Name          string
	CreatedAt     time.Time
}

type ObjectInfo struct {
	Name         string    `json:"name"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	LastModified time.Time `json:"last_modified"`
}

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

type UserStorageLimits struct {
	Containers int
	StorageGB  int
}
