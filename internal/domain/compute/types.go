package compute

import "time"

// Instance는 DB에 저장되는 불변 메타데이터.
// 가변 상태(status, ip)는 읽기 시 OpenStack에서 직접 조회한다.
type Instance struct {
	OpenstackID string
	Name        string
	Status      string
	ImageID     string
	FlavorID    string
	KeyName     string
	Note        string
	App         *AppSummary
	Created     time.Time
}

type AppSummary struct {
	ID        string `json:"id"`
	Subdomain string `json:"subdomain"`
	Host      string `json:"host"`
}

type Flavor struct {
	ID    string
	Name  string
	VCPUs int
	RAM   int
	Disk  int
}

type QuotaDetail struct {
	InUse    int
	Limit    int
	Reserved int
}

type QuotaDetailSet struct {
	Cores     QuotaDetail
	RAM       QuotaDetail
	Instances QuotaDetail
}

type UserUsageLimits struct {
	Instances int
	VCPUs     int
	RAMMB     int
	DiskGB    int
}

type UserUsage struct {
	Instances int
	VCPUs     int
	RAMMB     int
	DiskGB    int
}

type Server struct {
	ID             string
	Name           string
	Status         string
	Image          map[string]any
	Flavor         map[string]any
	Addresses      map[string]any
	KeyName        string
	SecurityGroups []map[string]any
	AccessIPv4     string
	Created        time.Time `json:"created"`
}

type NetworkID struct {
	UUID string
}

type CreateServerOpts struct {
	Name           string
	ImageRef       string
	FlavorRef      string
	KeyName        string
	Note           string
	SecurityGroups []string
	Networks       []NetworkID
}

// --- Request/Response DTOs ---

type FlavorResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`  // MB
	Disk  int    `json:"disk"` // GB
}

type AvailableFlavorResponse struct {
	FlavorResponse
	MaxConfigurable int `json:"max_configurable"`
}

type CreateInstanceRequest struct {
	Name           string   `json:"name" binding:"required"`
	ImageID        string   `json:"image_id" binding:"required"`
	FlavorID       string   `json:"flavor_id" binding:"required"`
	NetworkID      string   `json:"network_id"`
	KeyName        string   `json:"key_name"`
	Note           string   `json:"note"`
	SecurityGroups []string `json:"security_groups"`
}

type UpdateInstanceRequest struct {
	Name    string `json:"name"`
	KeyName string `json:"key_name"`
	Note    string `json:"note"`
}

type CreateInstanceResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	ImageID        string   `json:"image_id"`
	FlavorID       string   `json:"flavor_id"`
	KeyName        string   `json:"key_name"`
	SecurityGroups []string `json:"security_groups"`
	FixedIP        string   `json:"fixed_ip"`
	FloatingIP     string   `json:"floating_ip"`
}

type InstanceDetailResponse struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	Image      string         `json:"image"`
	Flavor     FlavorResponse `json:"flavor"`
	KeyName    string         `json:"key_name"`
	Note       string         `json:"note"`
	FixedIP    string         `json:"fixed_ip,omitempty"`
	FloatingIP string         `json:"floating_ip,omitempty"`
	App        *AppSummary    `json:"app,omitempty"`
	Usage      UsageStats     `json:"usage"`
	Created    time.Time      `json:"created"`
}

// UsageStats — OpenStack diagnostics API 기반. 모든 하이퍼바이저가 지원하지는 않는다.
type UsageStats struct {
	CPUUsage    float64 `json:"cpu_usage"`    // vCPU 시간 (누적)
	MemoryUsage int     `json:"memory_usage"` // MB
}

func ExtractFixedIP(server *Server) string {
	fixedIP, _ := extractServerIPs(server)
	return fixedIP
}
