package compute

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/diagnostics"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/hypervisors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type computeRepository interface {
	FetchFlavors() ([]flavors.Flavor, error)
	GetComputeQuota(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error)
	GetComputeClient() (*gophercloud.ServiceClient, error)
	CreateServer(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error)
	// 추가된 메소드
	FetchInstances() ([]servers.Server, error)
	FetchInstanceDetail(serverID string) (*servers.Server, map[string]interface{}, error)
	DeleteServer(client *gophercloud.ServiceClient, id string) error
	GetHypervisorList(client *gophercloud.ServiceClient) ([]hypervisors.Hypervisor, error)
}

type Repository struct {
	Client *gophercloud.ProviderClient
}

type CreateServerOpts struct {
	Name           string
	ImageRef       string
	FlavorRef      string
	KeyName        string
	SecurityGroups []string
	Networks       []servers.Network // 네트워크 정보가 없으면 생성이 안 될 수 있습니다.
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

func (r *Repository) FetchInstances() ([]servers.Server, error) {
	client, err := r.GetComputeClient()
	if err != nil {
		return nil, err
	}

	pager := servers.List(client, servers.ListOpts{})

	allPages, err := pager.AllPages()
	if err != nil {
		return nil, err
	}
	result, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// FetchInstanceDetail은 특정 VM의 상세 정보와 진단 데이터를 가져옵니다.
func (r *Repository) FetchInstanceDetail(serverID string) (*servers.Server, map[string]interface{}, error) {
	client, err := openstack.NewComputeV2(r.Client, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
	if err != nil {
		return nil, nil, err
	}

	// 1. 기본 서버 정보 조회
	server, err := servers.Get(client, serverID).Extract()
	if err != nil {
		return nil, nil, err
	}

	// 2. 실시간 사용량(Diagnostics) 조회
	// [수정 포인트] servers.GetDiagnostics (X) -> diagnostics.Get (O)
	diag, err := diagnostics.Get(client, serverID).Extract()
	if err != nil {
		// 진단 정보 조회 실패 시 기본 정보(server)만이라도 반환
		return server, nil, nil
	}

	return server, diag, nil
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
	baseOpts := servers.CreateOpts{
		Name:           opts.Name,
		ImageRef:       opts.ImageRef,
		FlavorRef:      opts.FlavorRef,
		SecurityGroups: opts.SecurityGroups,
		Networks:       opts.Networks,
	}

	createOpts := keypairs.CreateOptsExt{
		CreateOptsBuilder: baseOpts,
		KeyName:           opts.KeyName,
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
