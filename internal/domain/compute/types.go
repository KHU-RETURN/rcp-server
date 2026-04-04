package compute

// --- Domain models (gophercloud 의존 없음) ---

// Flavor는 OpenStack flavor의 도메인 표현입니다.
type Flavor struct {
	ID    string
	Name  string
	VCPUs int
	RAM   int
	Disk  int
}

// QuotaDetail은 단일 쿼터 항목의 상세 정보입니다.
type QuotaDetail struct {
	InUse    int
	Limit    int
	Reserved int
}

// QuotaDetailSet은 compute 쿼터 전체 정보입니다.
type QuotaDetailSet struct {
	Cores     QuotaDetail
	RAM       QuotaDetail
	Instances QuotaDetail
}

// Server는 생성된 서버의 도메인 표현입니다.
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
}

// NetworkID는 서버 생성 시 사용할 네트워크 식별자입니다.
type NetworkID struct {
	UUID string
}

// CreateServerOpts는 서버 생성 요청 옵션입니다.
type CreateServerOpts struct {
	Name           string
	ImageRef       string
	FlavorRef      string
	KeyName        string
	SecurityGroups []string
	Networks       []NetworkID
}

// --- Request/Response DTOs ---

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
