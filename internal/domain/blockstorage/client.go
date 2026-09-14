package blockstorage

import (
	"errors"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/volumeattach"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
)

type Client struct {
	provider *gophercloud.ProviderClient
}

func NewClient(provider *gophercloud.ProviderClient) *Client {
	return &Client{provider: provider}
}

func isOpenStackNotFound(err error) bool {
	var notFound gophercloud.ErrDefault404
	return errors.As(err, &notFound)
}

func (c *Client) blockStorageClient() (*gophercloud.ServiceClient, error) {
	return goopenstack.NewBlockStorageV3(c.provider, gophercloud.EndpointOpts{
		Region: openstack.Region,
	})
}

func (c *Client) computeClient() (*gophercloud.ServiceClient, error) {
	return goopenstack.NewComputeV2(c.provider, gophercloud.EndpointOpts{
		Region: openstack.Region,
	})
}

func (c *Client) ListVolumes(ownerID string) ([]Volume, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	pages, err := volumes.List(sc, volumes.ListOpts{
		Metadata: map[string]string{ownerMetadataKey: ownerID},
		Sort:     "created_at:desc",
	}).AllPages()
	if err != nil {
		return nil, err
	}
	raw, err := volumes.ExtractVolumes(pages)
	if err != nil {
		return nil, err
	}
	result := make([]Volume, len(raw))
	for i, volume := range raw {
		result[i] = convertVolume(volume)
	}
	return result, nil
}

func (c *Client) GetVolume(id string) (*Volume, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	raw, err := volumes.Get(sc, id).Extract()
	if err != nil {
		return nil, err
	}
	volume := convertVolume(*raw)
	return &volume, nil
}

func (c *Client) CreateVolume(req CreateVolumeRequest, ownerID string) (*Volume, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	raw, err := volumes.Create(sc, volumes.CreateOpts{
		Name:             req.Name,
		Description:      req.Description,
		Size:             req.SizeGiB,
		AvailabilityZone: req.AvailabilityZone,
		VolumeType:       req.VolumeType,
		SnapshotID:       req.SnapshotID,
		Metadata:         map[string]string{ownerMetadataKey: ownerID},
	}).Extract()
	if err != nil {
		return nil, err
	}
	volume := convertVolume(*raw)
	return &volume, nil
}

func (c *Client) UpdateVolume(id string, req UpdateVolumeRequest) (*Volume, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	raw, err := volumes.Update(sc, id, volumes.UpdateOpts{
		Name:        &req.Name,
		Description: &req.Description,
	}).Extract()
	if err != nil {
		return nil, err
	}
	volume := convertVolume(*raw)
	return &volume, nil
}

func (c *Client) DeleteVolume(id string) error {
	sc, err := c.blockStorageClient()
	if err != nil {
		return err
	}
	return volumes.Delete(sc, id, nil).ExtractErr()
}

func (c *Client) AttachVolume(instanceID, volumeID string) (*VolumeAttachment, error) {
	sc, err := c.computeClient()
	if err != nil {
		return nil, err
	}
	raw, err := volumeattach.Create(sc, instanceID, volumeattach.CreateOpts{VolumeID: volumeID}).Extract()
	if err != nil {
		return nil, err
	}
	return &VolumeAttachment{
		InstanceID: raw.ServerID,
		Device:     raw.Device,
	}, nil
}

func (c *Client) DetachVolume(instanceID, volumeID string) error {
	sc, err := c.computeClient()
	if err != nil {
		return err
	}
	return volumeattach.Delete(sc, instanceID, volumeID).ExtractErr()
}

func (c *Client) GetInstance(instanceID string) error {
	sc, err := c.computeClient()
	if err != nil {
		return err
	}
	_, err = servers.Get(sc, instanceID).Extract()
	return err
}

func (c *Client) ListSnapshots(ownerID string) ([]Snapshot, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	pages, err := snapshots.List(sc, snapshots.ListOpts{Sort: "created_at:desc"}).AllPages()
	if err != nil {
		return nil, err
	}
	raw, err := snapshots.ExtractSnapshots(pages)
	if err != nil {
		return nil, err
	}
	result := make([]Snapshot, 0, len(raw))
	for _, snapshot := range raw {
		converted := convertSnapshot(snapshot)
		if converted.Metadata[ownerMetadataKey] == ownerID {
			result = append(result, converted)
		}
	}
	return result, nil
}

func (c *Client) GetSnapshot(id string) (*Snapshot, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	raw, err := snapshots.Get(sc, id).Extract()
	if err != nil {
		return nil, err
	}
	snapshot := convertSnapshot(*raw)
	return &snapshot, nil
}

func (c *Client) CreateSnapshot(volumeID string, req CreateSnapshotRequest, ownerID string) (*Snapshot, error) {
	sc, err := c.blockStorageClient()
	if err != nil {
		return nil, err
	}
	raw, err := snapshots.Create(sc, snapshots.CreateOpts{
		VolumeID:    volumeID,
		Name:        req.Name,
		Description: req.Description,
		Metadata:    map[string]string{ownerMetadataKey: ownerID},
	}).Extract()
	if err != nil {
		return nil, err
	}
	snapshot := convertSnapshot(*raw)
	return &snapshot, nil
}

func (c *Client) DeleteSnapshot(id string) error {
	sc, err := c.blockStorageClient()
	if err != nil {
		return err
	}
	return snapshots.Delete(sc, id).ExtractErr()
}

func convertVolume(raw volumes.Volume) Volume {
	attachments := make([]VolumeAttachment, len(raw.Attachments))
	for i, attachment := range raw.Attachments {
		attachments[i] = VolumeAttachment{
			InstanceID: attachment.ServerID,
			Device:     attachment.Device,
			AttachedAt: attachment.AttachedAt,
		}
	}
	return Volume{
		ID:               raw.ID,
		Name:             raw.Name,
		Description:      raw.Description,
		SizeGiB:          raw.Size,
		Status:           raw.Status,
		Bootable:         raw.Bootable == "true",
		Encrypted:        raw.Encrypted,
		VolumeType:       raw.VolumeType,
		AvailabilityZone: raw.AvailabilityZone,
		CreatedAt:        raw.CreatedAt,
		Attachments:      attachments,
		Metadata:         raw.Metadata,
	}
}

func convertSnapshot(raw snapshots.Snapshot) Snapshot {
	return Snapshot{
		ID:          raw.ID,
		Name:        raw.Name,
		Description: raw.Description,
		VolumeID:    raw.VolumeID,
		SizeGiB:     raw.Size,
		Status:      raw.Status,
		CreatedAt:   raw.CreatedAt,
		Metadata:    raw.Metadata,
	}
}
