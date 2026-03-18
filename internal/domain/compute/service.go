package compute

// Service는 비즈니스 로직을 담당합니다.
type Service struct {
	Repo *Repository
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

// GetAvailableFlavors는 Repo에서 가져온 데이터를 우리 규격(FlavorResponse)으로 변환합니다.
func (s *Service) GetAvailableFlavors() ([]FlavorResponse, error) {
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

// GetInstances는 VM 전체 목록을 가져와 변환합니다.
func (s *Service) GetInstances() ([]InstanceDetailResponse, error) {
	rawServers, err := s.Repo.FetchInstances()
	if err != nil {
		return nil, err
	}

	var res []InstanceDetailResponse
	for _, srv := range rawServers {
		res = append(res, InstanceDetailResponse{
			ID:     srv.ID,
			Name:   srv.Name,
			Status: srv.Status,
			// 목록 조회 시에는 usage 정보를 생략하거나 0으로 세팅 (속도 때문)
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
        if cpu, ok := diag["cpu0_time"].(float64); ok { usage.CPUUsage = cpu }
        if mem, ok := diag["memory"].(float64); ok { usage.MemoryUsage = int(mem / 1024) }
    }

    // 6. 최종 합체
    return &InstanceDetailResponse{
        ID:        server.ID,
        Name:      server.Name,
        Status:    server.Status,
        Addresses: addrMap,
        Flavor:    targetFlavor,
        Usage:     usage,
    }, nil
}