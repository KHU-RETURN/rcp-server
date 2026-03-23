package compute

import "time"

// 기본 정보 (all 용)
type FlavorResponse struct {
	// Flavor UUID.
	ID string `json:"id"`
	// Human-readable flavor name.
	Name string `json:"name"`
	// Number of virtual CPUs.
	VCPUs int `json:"vcpus"`
	// Memory in MB.
	RAM int `json:"ram"` // MB 단위
	// Disk size in GB.
	Disk int `json:"disk"` // GB 단위
}

// 계산 정보 포함 (available 용) - 상속(Embedding) 활용
type AvailableFlavorResponse struct {
	FlavorResponse
	// Maximum number of instances that can still be configured with this flavor.
	MaxConfigurable int `json:"max_configurable"`
}

type CreateInstanceRequest struct {
	// Instance name.
	Name string `json:"name" binding:"required"`
	// OpenStack image UUID.
	ImageRef string `json:"image_id" binding:"required"`
	// OpenStack flavor UUID.
	FlavorRef string `json:"flavor_id" binding:"required"`
	// Optional OpenStack network UUID.
	NetworkID string `json:"network_id"`
}

type CreateInstanceResponse struct {
	ID              string                   `json:"id"`
	TenantID        string                   `json:"tenant_id"`
	UserID          string                   `json:"user_id"`
	Name            string                   `json:"name"`
	Updated         time.Time                `json:"updated"`
	Created         time.Time                `json:"created"`
	HostID          string                   `json:"hostid"`
	Status          string                   `json:"status"`
	Progress        int                      `json:"progress"`
	AccessIPv4      string                   `json:"accessIPv4"`
	AccessIPv6      string                   `json:"accessIPv6"`
	Flavor          map[string]interface{}   `json:"flavor"`
	Addresses       map[string]interface{}   `json:"addresses"`
	Metadata        map[string]string        `json:"metadata"`
	Links           []interface{}            `json:"links"`
	KeyName         string                   `json:"key_name"`
	AdminPass       string                   `json:"adminPass"`
	SecurityGroups  []map[string]interface{} `json:"security_groups"`
	AttachedVolumes []InstanceAttachedVolume `json:"os-extended-volumes:volumes_attached"`
	Fault           InstanceFault            `json:"fault"`
	Tags            *[]string                `json:"tags"`
	ServerGroups    *[]string                `json:"server_groups"`
}

type InstanceAttachedVolume struct {
	ID string `json:"id"`
}

type InstanceFault struct {
	Code    int       `json:"code"`
	Created time.Time `json:"created"`
	Details string    `json:"details"`
	Message string    `json:"message"`
}
