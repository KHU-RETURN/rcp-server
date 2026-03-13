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