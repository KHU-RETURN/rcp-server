package compute

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

// Service는 비즈니스 로직을 담당합니다.
type Service struct {
	Repo computeRepository
}

var (
	ErrCreateInstanceNameRequired   = errors.New("name is required")
	ErrCreateInstanceImageRequired  = errors.New("image_id is required")
	ErrCreateInstanceFlavorRequired = errors.New("flavor_id is required")
)

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo computeRepository) *Service {
	return &Service{Repo: repo}
}

// GetFlavors는 Repo에서 가져온 데이터를 우리 규격(FlavorResponse)으로 변환합니다.
func (s *Service) GetFlavors() ([]FlavorResponse, error) {
	rawFlavors, err := s.Repo.FetchFlavors()
	if err != nil {
		return nil, err
	}

	var res []FlavorResponse
	for _, f := range rawFlavors {
		res = append(res, FlavorResponse{
			ID:    f.ID,
			Name:  f.Name,
			VCPUs: f.VCPUs,
			RAM:   f.RAM,
			Disk:  f.Disk,
		})
	}
	return res, nil
}

// GetInstances는 VM 전체 목록을 가져와 변환하며, 각 VM의 Flavor 상세 정보도 포함합니다.
func (s *Service) GetInstances() ([]InstanceDetailResponse, error) {
	// 1. 모든 인스턴스 목록 가져오기
	rawServers, err := s.Repo.FetchInstances()
	if err != nil {
		return nil, err
	}

	// 2. [최적화] 모든 Flavor 정보를 미리 한 번만 가져와서 Map으로 변환
	// 루프 안에서 API를 계속 호출하는 것을 방지합니다.
	allFlavors, err := s.Repo.FetchFlavors()
	if err != nil {
		return nil, err
	}
	flavorMap := make(map[string]FlavorResponse)
	for _, f := range allFlavors {
		flavorMap[f.ID] = FlavorResponse{
			ID:    f.ID,
			Name:  f.Name,
			VCPUs: f.VCPUs,
			RAM:   f.RAM,
			Disk:  f.Disk,
		}
	}

	var res []InstanceDetailResponse

	for _, srv := range rawServers {
		// 3. IP 주소 파싱 (Helper 함수로 분리하면 더 깔끔합니다)
		addrMap := make(map[string]string)
		for netName, addrs := range srv.Addresses {
			if addrList, ok := addrs.([]interface{}); ok && len(addrList) > 0 {
				if firstAddr, ok := addrList[0].(map[string]interface{}); ok {
					if addr, ok := firstAddr["addr"].(string); ok {
						addrMap[netName] = addr
					}
				}
			}
		}

		// 4. 미리 만들어둔 flavorMap에서 해당 인스턴스의 Flavor 정보 찾기
		var targetFlavor FlavorResponse
		if srvFlavorID, ok := srv.Flavor["id"].(string); ok {
			if f, exists := flavorMap[srvFlavorID]; exists {
				targetFlavor = f
			}
		}

		// 5. 결과 리스트에 추가
		res = append(res, InstanceDetailResponse{
			ID:        srv.ID,
			Name:      srv.Name,
			Status:    srv.Status,
			Created:   srv.Created,
			Image:     extractResourceID(srv.Image),
			Addresses: addrMap,
			Flavor:    targetFlavor, // 이제 목록 조회에서도 Flavor 상세 정보가 포함됩니다.
		})
	}

	return res, nil
}

// GetAvailableFlavorsWithLimit 는 남은 자원량을 계산하여 각 Flavor별 가용 대수를 포함해 반환합니다.
func (s *Service) GetAvailableFlavorsWithLimit(client *gophercloud.ServiceClient, projectID string) ([]AvailableFlavorResponse, error) {
	// 1. 전체 Flavor 목록 가져오기
	rawFlavors, err := s.Repo.FetchFlavors()
	if err != nil {
		return nil, err
	}

	// 2. Repo를 통해 상세 쿼터(Detail) 가져오기
	quota, err := s.Repo.GetComputeQuota(client, projectID)
	if err != nil {
		return nil, fmt.Errorf("쿼터 조회 실패: %v", err)
	}

	// 3. 남은 자원 계산 (구조체 접근 방식 수정)
	// QuotaDetailSet은 Cores.Limit, Cores.InUse 식으로 되어 있습니다.
	remCores := quota.Cores.Limit - quota.Cores.InUse
	remRAM := quota.RAM.Limit - quota.RAM.InUse
	remInstances := quota.Instances.Limit - quota.Instances.InUse

	var res []AvailableFlavorResponse
	for _, f := range rawFlavors {
		// vCPU 기준 가용 대수
		countByCPU := 0
		if f.VCPUs > 0 {
			countByCPU = remCores / f.VCPUs
		} else {
			countByCPU = 999 // vCPU가 0인 경우(거의 없지만) 예외 처리
		}

		// RAM 기준 가용 대수
		countByRAM := 0
		if f.RAM > 0 {
			countByRAM = remRAM / f.RAM
		} else {
			countByRAM = 999
		}

		// 4. 세 가지 제약(CPU, RAM, 총 Instance 개수) 중 가장 작은 값이 진짜 한도
		maxPossible := max(
			// 결과가 마이너스면 0으로 세팅
			min(remInstances, min(countByRAM, countByCPU)), 0)

		res = append(res, AvailableFlavorResponse{
			FlavorResponse: FlavorResponse{
				ID:    f.ID,
				Name:  f.Name,
				VCPUs: f.VCPUs,
				RAM:   f.RAM,
				Disk:  f.Disk,
			},
			MaxConfigurable: maxPossible,
		})
	}
	return res, nil
}
func (s *Service) GetInstanceDetail(id string) (*InstanceDetailResponse, error) {
	// 1. 리포지토리 호출 (서버 정보 + 진단 정보)
	server, diag, err := s.Repo.FetchInstanceDetail(id)
	if err != nil {
		return nil, err
	}

	// 2. [핵심] 리포지토리에 이미 있는 FetchFlavors를 써서 모든 사양 정보를 가져옴
	allFlavors, _ := s.Repo.FetchFlavors()

	// 3. 서버가 쓰고 있는 Flavor ID랑 일치하는 녀석을 찾아서 상세 정보 추출
	var targetFlavor FlavorResponse
	serverFlavorID := server.Flavor["id"].(string)

	for _, f := range allFlavors {
		if f.ID == serverFlavorID {
			targetFlavor = FlavorResponse{
				ID:    f.ID,
				Name:  f.Name,
				VCPUs: f.VCPUs, // 이제 여기서 0이 아닌 진짜 숫자가 담김!
				RAM:   f.RAM,
				Disk:  f.Disk,
			}
			break
		}
	}

	// 4. IP 주소 파싱
	addrMap := make(map[string]string)
	for netName, addrs := range server.Addresses {
		if addrList, ok := addrs.([]interface{}); ok && len(addrList) > 0 {
			if firstAddr, ok := addrList[0].(map[string]interface{}); ok {
				addrMap[netName] = firstAddr["addr"].(string)
			}
		}
	}

	// 5. 사용량 정보 매핑
	usage := UsageStats{}
	if diag != nil {
		if cpu, ok := diag["cpu0_time"].(float64); ok {
			usage.CPUUsage = cpu
		}
		if mem, ok := diag["memory"].(float64); ok {
			usage.MemoryUsage = int(mem / 1024)
		}
	}

	// 6. 최종 합체
	return &InstanceDetailResponse{
		ID:        server.ID,
		Name:      server.Name,
		Status:    server.Status,
		Addresses: addrMap,
		Flavor:    targetFlavor,
		Usage:     usage,
		Created:   server.Created,
		Image:     extractResourceID(server.Image),
	}, nil
}

func (s *Service) GetComputeClient() (*gophercloud.ServiceClient, error) {
	return s.Repo.GetComputeClient()
}

func (s *Service) CreateInstance(client *gophercloud.ServiceClient, opts CreateServerOpts) (*CreateInstanceResponse, error) {
	normalizedOpts := normalizeCreateServerOpts(opts)
	if err := validateCreateServerOpts(normalizedOpts); err != nil {
		return nil, err
	}

	server, err := s.Repo.CreateServer(client, normalizedOpts)
	if err != nil {
		return nil, err
	}

	return buildCreateInstanceResponse(server, normalizedOpts), nil
}

type serverAddress struct {
	Address string `json:"addr"`
	Type    string `json:"OS-EXT-IPS:type"`
}

func buildCreateServerOpts(req CreateInstanceRequest) (CreateServerOpts, error) {
	req = normalizeCreateInstanceRequest(req)

	opts := CreateServerOpts{
		Name:           req.Name,
		ImageRef:       req.ImageID,
		FlavorRef:      req.FlavorID,
		KeyName:        req.KeyName,
		SecurityGroups: req.SecurityGroups,
	}

	if req.NetworkID != "" {
		opts.Networks = []servers.Network{{UUID: req.NetworkID}}
	}

	if err := validateCreateServerOpts(opts); err != nil {
		return CreateServerOpts{}, err
	}

	return opts, nil
}

func normalizeCreateInstanceRequest(req CreateInstanceRequest) CreateInstanceRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.ImageID = strings.TrimSpace(req.ImageID)
	req.FlavorID = strings.TrimSpace(req.FlavorID)
	req.NetworkID = strings.TrimSpace(req.NetworkID)
	req.KeyName = strings.TrimSpace(req.KeyName)
	req.SecurityGroups = normalizeStringSlice(req.SecurityGroups)
	return req
}

func normalizeCreateServerOpts(opts CreateServerOpts) CreateServerOpts {
	opts.Name = strings.TrimSpace(opts.Name)
	opts.ImageRef = strings.TrimSpace(opts.ImageRef)
	opts.FlavorRef = strings.TrimSpace(opts.FlavorRef)
	opts.KeyName = strings.TrimSpace(opts.KeyName)
	opts.SecurityGroups = normalizeStringSlice(opts.SecurityGroups)

	networks := make([]servers.Network, 0, len(opts.Networks))
	for _, network := range opts.Networks {
		network.UUID = strings.TrimSpace(network.UUID)
		network.Port = strings.TrimSpace(network.Port)
		network.FixedIP = strings.TrimSpace(network.FixedIP)
		network.Tag = strings.TrimSpace(network.Tag)
		if network.UUID == "" && network.Port == "" && network.FixedIP == "" && network.Tag == "" {
			continue
		}
		networks = append(networks, network)
	}
	opts.Networks = networks

	return opts
}

func validateCreateServerOpts(opts CreateServerOpts) error {
	switch {
	case strings.TrimSpace(opts.Name) == "":
		return ErrCreateInstanceNameRequired
	case strings.TrimSpace(opts.ImageRef) == "":
		return ErrCreateInstanceImageRequired
	case strings.TrimSpace(opts.FlavorRef) == "":
		return ErrCreateInstanceFlavorRequired
	default:
		return nil
	}
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func buildCreateInstanceResponse(server *servers.Server, opts CreateServerOpts) *CreateInstanceResponse {
	fixedIP, floatingIP := extractServerIPs(server)
	keyName := firstNonEmpty(strings.TrimSpace(server.KeyName), opts.KeyName)
	securityGroups := extractSecurityGroupNames(server.SecurityGroups, opts.SecurityGroups)

	return &CreateInstanceResponse{
		ID:             server.ID,
		Name:           firstNonEmpty(strings.TrimSpace(server.Name), opts.Name),
		Status:         strings.TrimSpace(server.Status),
		ImageID:        firstNonEmpty(extractResourceID(server.Image), opts.ImageRef),
		FlavorID:       firstNonEmpty(extractResourceID(server.Flavor), opts.FlavorRef),
		KeyName:        keyName,
		SecurityGroups: securityGroups,
		FixedIP:        fixedIP,
		FloatingIP:     floatingIP,
	}
}

func extractServerIPs(server *servers.Server) (string, string) {
	var fixedIP string
	var floatingIP string

	for _, rawAddresses := range server.Addresses {
		addresses := decodeServerAddresses(rawAddresses)
		for _, address := range addresses {
			ip := strings.TrimSpace(address.Address)
			if ip == "" {
				continue
			}

			switch strings.TrimSpace(address.Type) {
			case "floating":
				if floatingIP == "" {
					floatingIP = ip
				}
			case "fixed":
				if fixedIP == "" {
					fixedIP = ip
				}
			}
		}
	}

	if floatingIP == "" {
		floatingIP = strings.TrimSpace(server.AccessIPv4)
	}

	return fixedIP, floatingIP
}

func decodeServerAddresses(rawAddresses any) []serverAddress {
	if rawAddresses == nil {
		return nil
	}

	payload, err := json.Marshal(rawAddresses)
	if err != nil {
		return nil
	}

	var addresses []serverAddress
	if err := json.Unmarshal(payload, &addresses); err != nil {
		return nil
	}

	return addresses
}

func extractResourceID(resource map[string]any) string {
	if resource == nil {
		return ""
	}

	if id, ok := resource["id"].(string); ok {
		return strings.TrimSpace(id)
	}

	return ""
}

func extractSecurityGroupNames(groups []map[string]any, fallback []string) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		name, _ := group["name"].(string)
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}

	if len(names) > 0 {
		return names
	}

	if len(fallback) == 0 {
		return nil
	}

	cloned := make([]string, len(fallback))
	copy(cloned, fallback)
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}
