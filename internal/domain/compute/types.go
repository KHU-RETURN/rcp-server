package compute

// 기본 정보 (all 용)
type FlavorResponse struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    VCPUs int    `json:"vcpus"`
    RAM   int    `json:"ram"`
    Disk  int    `json:"disk"`
}

// 계산 정보 포함 (available 용) - 상속(Embedding) 활용
type AvailableFlavorResponse struct {
    FlavorResponse
    MaxConfigurable int `json:"max_configurable"`
}