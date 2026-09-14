package blockstorage

import "time"

const ownerMetadataKey = "rcp_owner_id"

type VolumeAttachment struct {
	InstanceID   string    `json:"instanceId"`
	InstanceName string    `json:"instanceName,omitempty"`
	Device       string    `json:"device,omitempty"`
	AttachedAt   time.Time `json:"attachedAt,omitempty"`
}

type Volume struct {
	ID               string
	Name             string
	Description      string
	SizeGiB          int
	Status           string
	Bootable         bool
	Encrypted        bool
	VolumeType       string
	AvailabilityZone string
	CreatedAt        time.Time
	Attachments      []VolumeAttachment
	Metadata         map[string]string
}

type Snapshot struct {
	ID          string
	Name        string
	Description string
	VolumeID    string
	SizeGiB     int
	Status      string
	CreatedAt   time.Time
	Metadata    map[string]string
}

type CreateVolumeRequest struct {
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	SizeGiB          int    `json:"sizeGiB" binding:"required"`
	VolumeType       string `json:"volumeType"`
	AvailabilityZone string `json:"availabilityZone"`
	SnapshotID       string `json:"snapshotId"`
}

type UpdateVolumeRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AttachVolumeRequest struct {
	InstanceID string `json:"instanceId" binding:"required"`
}

type CreateSnapshotRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type VolumeResponse struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	SizeGiB          int                `json:"sizeGiB"`
	Status           string             `json:"status"`
	Bootable         bool               `json:"bootable"`
	Encrypted        bool               `json:"encrypted"`
	VolumeType       string             `json:"volumeType"`
	AvailabilityZone string             `json:"availabilityZone"`
	CreatedAt        time.Time          `json:"createdAt"`
	Attachments      []VolumeAttachment `json:"attachments"`
}

type VolumeListResponse struct {
	Volumes []VolumeResponse `json:"volumes"`
}

type SnapshotResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	VolumeID    string    `json:"volumeId"`
	SizeGiB     int       `json:"sizeGiB"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SnapshotListResponse struct {
	Snapshots []SnapshotResponse `json:"snapshots"`
}

type AttachmentResponse struct {
	VolumeID   string `json:"volumeId"`
	InstanceID string `json:"instanceId"`
	Status     string `json:"status"`
}
