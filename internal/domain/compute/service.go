package compute

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type computeRepository interface {
	FetchFlavors() ([]flavors.Flavor, error)
	GetComputeQuota(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error)
	GetComputeClient() (*gophercloud.ServiceClient, error)
	CreateServer(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error)
}

// Service는 비즈니스 로직을 담당합니다.
type Service struct {
	Repo computeRepository
}

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

func (s *Service) CreateInstance(client *gophercloud.ServiceClient, opts CreateServerOpts) (*CreateInstanceResponse, error) {
	server, err := s.Repo.CreateServer(client, opts)
	if err != nil {
		return nil, err
	}

	return mapCreateInstanceResponse(server), nil
}

func mapCreateInstanceResponse(server *servers.Server) *CreateInstanceResponse {
	if server == nil {
		return nil
	}

	return &CreateInstanceResponse{
		ID:              server.ID,
		TenantID:        server.TenantID,
		UserID:          server.UserID,
		Name:            server.Name,
		Updated:         server.Updated,
		Created:         server.Created,
		HostID:          server.HostID,
		Status:          server.Status,
		Progress:        server.Progress,
		AccessIPv4:      server.AccessIPv4,
		AccessIPv6:      server.AccessIPv6,
		Flavor:          server.Flavor,
		Addresses:       server.Addresses,
		Metadata:        server.Metadata,
		Links:           server.Links,
		KeyName:         server.KeyName,
		AdminPass:       server.AdminPass,
		SecurityGroups:  server.SecurityGroups,
		AttachedVolumes: mapAttachedVolumes(server.AttachedVolumes),
		Fault:           mapFault(server.Fault),
		Tags:            server.Tags,
		ServerGroups:    server.ServerGroups,
	}
}

func mapAttachedVolumes(volumes []servers.AttachedVolume) []InstanceAttachedVolume {
	if volumes == nil {
		return nil
	}

	res := make([]InstanceAttachedVolume, 0, len(volumes))
	for _, volume := range volumes {
		res = append(res, InstanceAttachedVolume{
			ID: volume.ID,
		})
	}

	return res
}

func mapFault(fault servers.Fault) InstanceFault {
	return InstanceFault{
		Code:    fault.Code,
		Created: fault.Created,
		Details: fault.Details,
		Message: fault.Message,
	}
}
