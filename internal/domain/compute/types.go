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

// CreateInstanceRequest는 VM 생성 요청 본문입니다.
type CreateInstanceRequest struct {
	Name           string   `json:"name" binding:"required"`
	ImageID        string   `json:"image_id" binding:"required"`
	FlavorID       string   `json:"flavor_id" binding:"required"`
	NetworkID      string   `json:"network_id"`
	KeyName        string   `json:"key_name"`
	SecurityGroups []string `json:"security_groups"`
}

// CreateInstanceResponse는 VM 생성 성공 응답 규격입니다.
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

//조회

// InstanceDetailResponse는 VM의 상세 정보와 실시간 사용량을 담습니다.
type InstanceDetailResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Addresses map[string]string `json:"addresses"` // 네트워크 정보
	Flavor    FlavorResponse    `json:"flavor"`    // 할당된 전체 사양
	Usage     UsageStats        `json:"usage"`     // 실시간 사용량
	Created   time.Time         `json:"created"`
	Image     string            `json:"image"`
}

// 현재 사용중인 자원량
type UsageStats struct {
	CPUUsage    float64 `json:"cpu_usage"`    // % 단위 또는 vCPU 시간
	MemoryUsage int     `json:"memory_usage"` // MB 단위
	DiskUsage   int     `json:"disk_usage"`   // GB 단위
}
