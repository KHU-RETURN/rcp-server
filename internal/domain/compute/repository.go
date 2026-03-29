package compute

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/hypervisors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type Repository struct {
	Client *gophercloud.ProviderClient
}

type CreateServerOpts struct {
	Name      string
	ImageRef  string
	FlavorRef string
	Networks  []servers.Network // 네트워크 정보가 없으면 생성이 안 될 수 있습니다.
}

func NewRepository(client *gophercloud.ProviderClient) *Repository {
	return &Repository{Client: client}
}

// GetComputeClient - 외부 패키지 안 쓰고 리포지토리 자체 클라이언트로 서비스 클라이언트 생성
func (r *Repository) GetComputeClient() (*gophercloud.ServiceClient, error) {
	// 리포지토리가 들고 있는 r.Client(Provider)를 사용해서 바로 생성합니다.
	return openstack.NewComputeV2(r.Client, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
}

// FetchFlavors - 여기서도 직접 r.GetComputeClient()를 호출해서 쓰면 중복 코드 줄어듭니다.
func (r *Repository) FetchFlavors() ([]flavors.Flavor, error) {
	client, err := r.GetComputeClient() // 방금 위에서 만든 함수 활용
	if err != nil {
		return nil, err
	}

	allPages, err := flavors.ListDetail(client, nil).AllPages()
	if err != nil {
		return nil, err
	}

	return flavors.ExtractFlavors(allPages)
}

// GetComputeQuota - 서비스 클라이언트를 인자로 받아서 쿼터 상세 정보 조회
func (r *Repository) GetComputeQuota(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error) {
	detail, err := quotasets.GetDetail(client, projectID).Extract()
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

func (r *Repository) CreateServer(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
	// 오픈스택 SDK 규격에 맞게 옵션 설정
	createOpts := servers.CreateOpts{
		Name:      opts.Name,
		ImageRef:  opts.ImageRef,
		FlavorRef: opts.FlavorRef,
		Networks:  opts.Networks,
	}

	// 실제 생성 요청 보내기
	server, err := servers.Create(client, createOpts).Extract()
	if err != nil {
		return nil, err
	}
	return server, nil
}

// GetHypervisorList - 모든 하이퍼바이저의 상세 정보 조회
func (r *Repository) GetHypervisorList(client *gophercloud.ServiceClient) ([]hypervisors.Hypervisor, error) {
	// 두 번째 인자로 nil을 넘겨서 기본 리스트 옵션을 사용합니다.
	allPages, err := hypervisors.List(client, nil).AllPages()
	if err != nil {
		return nil, err
	}

	allHypervisors, err := hypervisors.ExtractHypervisors(allPages)
	if err != nil {
		return nil, err
	}

	return allHypervisors, nil
}

func (r *Repository) DeleteServer(client *gophercloud.ServiceClient, id string) error {
	// ID를 받아서 해당 서버를 삭제 요청합니다.
	return servers.Delete(client, id).ExtractErr()
}
