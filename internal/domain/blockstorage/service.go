package blockstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type blockStorageClient interface {
	ListVolumes(ownerID string) ([]Volume, error)
	GetVolume(id string) (*Volume, error)
	CreateVolume(req CreateVolumeRequest, ownerID string) (*Volume, error)
	UpdateVolume(id string, req UpdateVolumeRequest) (*Volume, error)
	DeleteVolume(id string) error
	AttachVolume(instanceID, volumeID string) (*VolumeAttachment, error)
	DetachVolume(instanceID, volumeID string) error
	GetInstance(instanceID string) error
	ListSnapshots(ownerID string) ([]Snapshot, error)
	GetSnapshot(id string) (*Snapshot, error)
	CreateSnapshot(volumeID string, req CreateSnapshotRequest, ownerID string) (*Snapshot, error)
	DeleteSnapshot(id string) error
}

type instanceRepository interface {
	FindInstanceName(ctx context.Context, ownerID uuid.UUID, openstackID string) (string, bool, error)
	InstanceExists(ctx context.Context, ownerID uuid.UUID, openstackID string) (bool, error)
}

type Service struct {
	client blockStorageClient
	repo   instanceRepository
}

var (
	ErrVolumeNotFound              = errors.New("volume not found")
	ErrSnapshotNotFound            = errors.New("snapshot not found")
	ErrInstanceNotFound            = errors.New("instance not found")
	ErrVolumeNameRequired          = errors.New("volume name is required")
	ErrVolumeSizeInvalid           = errors.New("volume size must be greater than zero")
	ErrSnapshotNameRequired        = errors.New("snapshot name is required")
	ErrVolumeNotAvailable          = errors.New("volume must be available")
	ErrVolumeAttached              = errors.New("volume is attached")
	ErrSnapshotBusy                = errors.New("snapshot is busy")
	ErrSnapshotSmallerThanSource   = errors.New("volume size cannot be smaller than snapshot size")
	ErrVolumeAttachmentNotFound    = errors.New("volume attachment not found")
	ErrBlockStorageOperationFailed = errors.New("block storage operation failed")
)

func NewService(client blockStorageClient, repo instanceRepository) *Service {
	return &Service{client: client, repo: repo}
}

func (s *Service) ListVolumes(ctx context.Context, ownerID uuid.UUID) ([]VolumeResponse, error) {
	raw, err := s.client.ListVolumes(ownerID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	result := make([]VolumeResponse, len(raw))
	for i, volume := range raw {
		result[i] = s.volumeResponse(ctx, ownerID, volume)
	}
	return result, nil
}

func (s *Service) GetVolume(ctx context.Context, ownerID uuid.UUID, id string) (*VolumeResponse, error) {
	volume, err := s.resolveVolume(ownerID, id)
	if err != nil {
		return nil, err
	}
	response := s.volumeResponse(ctx, ownerID, *volume)
	return &response, nil
}

func (s *Service) CreateVolume(ctx context.Context, ownerID uuid.UUID, req CreateVolumeRequest) (*VolumeResponse, error) {
	req = normalizeCreateVolumeRequest(req)
	if err := validateCreateVolumeRequest(req); err != nil {
		return nil, err
	}
	if req.SnapshotID != "" {
		snapshot, err := s.resolveSnapshot(ownerID, req.SnapshotID)
		if err != nil {
			return nil, err
		}
		if req.SizeGiB < snapshot.SizeGiB {
			return nil, ErrSnapshotSmallerThanSource
		}
	}
	volume, err := s.client.CreateVolume(req, ownerID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	response := s.volumeResponse(ctx, ownerID, *volume)
	return &response, nil
}

func (s *Service) UpdateVolume(ctx context.Context, ownerID uuid.UUID, id string, req UpdateVolumeRequest) (*VolumeResponse, error) {
	volume, err := s.resolveVolume(ownerID, id)
	if err != nil {
		return nil, err
	}
	req = normalizeUpdateVolumeRequest(req, *volume)
	updated, err := s.client.UpdateVolume(id, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	response := s.volumeResponse(ctx, ownerID, *updated)
	return &response, nil
}

func (s *Service) DeleteVolume(ownerID uuid.UUID, id string) error {
	volume, err := s.resolveVolume(ownerID, id)
	if err != nil {
		return err
	}
	if volume.Status != "available" {
		return ErrVolumeAttached
	}
	if err := s.client.DeleteVolume(id); err != nil {
		return fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) AttachVolume(ctx context.Context, ownerID uuid.UUID, volumeID, instanceID string) (*AttachmentResponse, error) {
	instanceID = strings.TrimSpace(instanceID)
	volume, err := s.resolveVolume(ownerID, volumeID)
	if err != nil {
		return nil, err
	}
	if volume.Status != "available" {
		return nil, ErrVolumeNotAvailable
	}
	ok, err := s.repo.InstanceExists(ctx, ownerID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	if !ok {
		return nil, ErrInstanceNotFound
	}
	if err := s.client.GetInstance(instanceID); err != nil {
		if isOpenStackNotFound(err) {
			return nil, ErrInstanceNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	if _, err := s.client.AttachVolume(instanceID, volumeID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	return &AttachmentResponse{VolumeID: volumeID, InstanceID: instanceID, Status: "attaching"}, nil
}

func (s *Service) DetachVolume(ctx context.Context, ownerID uuid.UUID, volumeID, instanceID string) (*AttachmentResponse, error) {
	volume, err := s.resolveVolume(ownerID, volumeID)
	if err != nil {
		return nil, err
	}
	attached := false
	for _, attachment := range volume.Attachments {
		if attachment.InstanceID == instanceID {
			attached = true
			break
		}
	}
	if !attached {
		return nil, ErrVolumeAttachmentNotFound
	}
	ok, err := s.repo.InstanceExists(ctx, ownerID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	if !ok {
		return nil, ErrInstanceNotFound
	}
	if err := s.client.DetachVolume(instanceID, volumeID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	return &AttachmentResponse{VolumeID: volumeID, InstanceID: instanceID, Status: "detaching"}, nil
}

func (s *Service) ListSnapshots(ownerID uuid.UUID) ([]SnapshotResponse, error) {
	raw, err := s.client.ListSnapshots(ownerID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	result := make([]SnapshotResponse, len(raw))
	for i, snapshot := range raw {
		result[i] = snapshotResponse(snapshot)
	}
	return result, nil
}

func (s *Service) CreateSnapshot(ownerID uuid.UUID, volumeID string, req CreateSnapshotRequest) (*SnapshotResponse, error) {
	req = normalizeCreateSnapshotRequest(req)
	if req.Name == "" {
		return nil, ErrSnapshotNameRequired
	}
	volume, err := s.resolveVolume(ownerID, volumeID)
	if err != nil {
		return nil, err
	}
	if volume.Status != "available" {
		return nil, ErrVolumeNotAvailable
	}
	snapshot, err := s.client.CreateSnapshot(volumeID, req, ownerID.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	response := snapshotResponse(*snapshot)
	return &response, nil
}

func (s *Service) DeleteSnapshot(ownerID uuid.UUID, id string) error {
	snapshot, err := s.resolveSnapshot(ownerID, id)
	if err != nil {
		return err
	}
	if snapshot.Status != "available" && snapshot.Status != "error" {
		return ErrSnapshotBusy
	}
	if err := s.client.DeleteSnapshot(id); err != nil {
		return fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	return nil
}

func (s *Service) resolveVolume(ownerID uuid.UUID, id string) (*Volume, error) {
	volume, err := s.client.GetVolume(strings.TrimSpace(id))
	if err != nil {
		if isOpenStackNotFound(err) {
			return nil, ErrVolumeNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	if volume.Metadata[ownerMetadataKey] != ownerID.String() {
		return nil, ErrVolumeNotFound
	}
	return volume, nil
}

func (s *Service) resolveSnapshot(ownerID uuid.UUID, id string) (*Snapshot, error) {
	snapshot, err := s.client.GetSnapshot(strings.TrimSpace(id))
	if err != nil {
		if isOpenStackNotFound(err) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrBlockStorageOperationFailed, err)
	}
	if snapshot.Metadata[ownerMetadataKey] != ownerID.String() {
		return nil, ErrSnapshotNotFound
	}
	return snapshot, nil
}

func (s *Service) volumeResponse(ctx context.Context, ownerID uuid.UUID, volume Volume) VolumeResponse {
	attachments := make([]VolumeAttachment, len(volume.Attachments))
	for i, attachment := range volume.Attachments {
		attachments[i] = attachment
		if name, ok, err := s.repo.FindInstanceName(ctx, ownerID, attachment.InstanceID); err == nil && ok {
			attachments[i].InstanceName = name
		}
	}
	return VolumeResponse{
		ID:               volume.ID,
		Name:             volume.Name,
		Description:      volume.Description,
		SizeGiB:          volume.SizeGiB,
		Status:           volume.Status,
		Bootable:         volume.Bootable,
		Encrypted:        volume.Encrypted,
		VolumeType:       volume.VolumeType,
		AvailabilityZone: volume.AvailabilityZone,
		CreatedAt:        volume.CreatedAt,
		Attachments:      attachments,
	}
}

func snapshotResponse(snapshot Snapshot) SnapshotResponse {
	return SnapshotResponse{
		ID:          snapshot.ID,
		Name:        snapshot.Name,
		Description: snapshot.Description,
		VolumeID:    snapshot.VolumeID,
		SizeGiB:     snapshot.SizeGiB,
		Status:      snapshot.Status,
		CreatedAt:   snapshot.CreatedAt,
	}
}

func normalizeCreateVolumeRequest(req CreateVolumeRequest) CreateVolumeRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.VolumeType = strings.TrimSpace(req.VolumeType)
	req.AvailabilityZone = strings.TrimSpace(req.AvailabilityZone)
	req.SnapshotID = strings.TrimSpace(req.SnapshotID)
	return req
}

func normalizeUpdateVolumeRequest(req UpdateVolumeRequest, current Volume) UpdateVolumeRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		req.Name = current.Name
	}
	return req
}

func normalizeCreateSnapshotRequest(req CreateSnapshotRequest) CreateSnapshotRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	return req
}

func validateCreateVolumeRequest(req CreateVolumeRequest) error {
	switch {
	case req.Name == "":
		return ErrVolumeNameRequired
	case req.SizeGiB <= 0:
		return ErrVolumeSizeInvalid
	default:
		return nil
	}
}
