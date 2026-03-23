package compute

import (
	"fmt"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"log"
)

// Service는 비즈니스 로직을 담당합니다.
type Service struct {
	Repo *Repository
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo *Repository) *Service {
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
		maxPossible := min(countByRAM, countByCPU)
		if remInstances < maxPossible {
			maxPossible = remInstances
		}

		// 결과가 마이너스면 0으로 세팅
		if maxPossible < 0 {
			maxPossible = 0
		}

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

func (s *Service) GetComputeClient() (*gophercloud.ServiceClient, error) {
	return s.Repo.GetComputeClient()
}

func (s *Service) CreateInstance(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
	// [Guard 1] 자원 체크 (CPU, RAM)
	if err := s.checkQuota(client, opts.FlavorRef); err != nil {
		return nil, err
	}

	// [Guard 2] 이름 중복 체크
	if err := s.checkDuplicateName(client, opts.Name); err != nil {
		return nil, err
	}

	// 모든 관문을 통과하면 생성 진행
	return s.Repo.CreateServer(client, opts)
}

func (s *Service) checkQuota(client *gophercloud.ServiceClient, flavorID string) error {
	// 1. Flavor 정보 가져오기
	flavor, err := flavors.Get(client, flavorID).Extract()
	if err != nil {
		return fmt.Errorf("flavor 정보를 확인할 수 없습니다: %v", err)
	}

	// 2. [Guard B] 물리 자원(Hypervisor) 실점유율 체크
	hvs, err := s.Repo.GetHypervisorList(client)
	if err != nil {
		return fmt.Errorf("하이퍼바이저 정보를 가져올 수 없습니다: %v", err)
	}

	var totalFreeVCPUs int
	var totalFreeRAM int

	// 모든 하이퍼바이저의 남은 자원을 합산합니다 (멀티 노드 대응)
	for _, hv := range hvs {
		totalFreeVCPUs += (hv.VCPUs - hv.VCPUsUsed)
		totalFreeRAM += hv.FreeRamMB
	}

	log.Printf("[DEBUG] 가용 물리 자원 합계 - CPU: %d, RAM: %dMB", totalFreeVCPUs, totalFreeRAM)

	if flavor.VCPUs > totalFreeVCPUs {
		return fmt.Errorf("물리 서버 CPU 부족 (필요: %d, 가용: %d)", flavor.VCPUs, totalFreeVCPUs)
	}
	if flavor.RAM > totalFreeRAM {
		return fmt.Errorf("물리 서버 RAM 부족 (필요: %dMB, 가용: %dMB)", flavor.RAM, totalFreeRAM)
	}

	return nil
}

// checkDuplicateName: 동일한 이름의 서버가 이미 존재하는지 확인
func (s *Service) checkDuplicateName(client *gophercloud.ServiceClient, name string) error {
	allPages, err := servers.List(client, servers.ListOpts{Name: name}).AllPages()
	if err != nil {
		return fmt.Errorf("서버 목록 조회 실패: %v", err)
	}

	allServers, err := servers.ExtractServers(allPages)
	if err != nil {
		return err
	}

	// 이름이 완전히 일치하는 서버가 있는지 체크
	for _, srv := range allServers {
		if srv.Name == name {
			return fmt.Errorf("이미 '%s'라는 이름의 서버가 존재합니다", name)
		}
	}

	return nil
}

func (s *Service) DeleteInstance(client *gophercloud.ServiceClient, id string) error {
	// [Guard] 삭제 전 서버가 존재하는지 확인
	_, err := servers.Get(client, id).Extract()
	if err != nil {
		return fmt.Errorf("삭제하려는 서버를 찾을 수 없습니다 (ID: %s)", id)
	}

	// 삭제 실행
	err = s.Repo.DeleteServer(client, id)
	if err != nil {
		return fmt.Errorf("서ver 삭제 실패: %v", err)
	}

	return nil
}
