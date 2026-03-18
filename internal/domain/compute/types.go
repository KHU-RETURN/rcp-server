package compute

// FlavorResponse는 프론트엔드에 전달할 사양 정보
type FlavorResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`  // MB 단위
	Disk  int    `json:"disk"` // GB 단위
}

// InstanceDetailResponse는 VM의 상세 정보와 실시간 사용량을 담습니다.
type InstanceDetailResponse struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Addresses map[string]string  `json:"addresses"` // 네트워크 정보
	Flavor    FlavorResponse     `json:"flavor"`    // 할당된 전체 사양
	Usage     UsageStats         `json:"usage"`     // 실시간 사용량
}

//현재 사용중인 자원량
type UsageStats struct {
	CPUUsage    float64 `json:"cpu_usage"`    // % 단위 또는 vCPU 시간
	MemoryUsage int     `json:"memory_usage"` // MB 단위
	DiskUsage   int     `json:"disk_usage"`   // GB 단위
}