package compute

import "time"

// 기본 정보 (all 용)
type FlavorResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`  // MB 단위
	Disk  int    `json:"disk"` // GB 단위
}

// 계산 정보 포함 (available 용) - 상속(Embedding) 활용
type AvailableFlavorResponse struct {
	FlavorResponse
	MaxConfigurable int `json:"max_configurable"`
}

type CreateInstanceRequest struct {
	Name      string `json:"name" binding:"required"`
	ImageRef  string `json:"image_id" binding:"required"`
	FlavorRef string `json:"flavor_id" binding:"required"`
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
	Flavor          map[string]any           `json:"flavor"`
	Addresses       map[string]any           `json:"addresses"`
	Metadata        map[string]string        `json:"metadata"`
	Links           []any                    `json:"links"`
	KeyName         string                   `json:"key_name"`
	AdminPass       string                   `json:"adminPass"`
	SecurityGroups  []map[string]any         `json:"security_groups"`
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
