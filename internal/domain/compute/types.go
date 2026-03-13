package compute

// FlavorResponse는 프론트엔드에 전달할 사양 정보
type FlavorResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`  // MB 단위
	Disk  int    `json:"disk"` // GB 단위
}