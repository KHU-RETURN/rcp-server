package compute

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/diagnostics"
)

type Repository struct {
	Client *gophercloud.ProviderClient
}

func NewRepository(client *gophercloud.ProviderClient) *Repository {
	return &Repository{Client: client}
}

// FetchFlavors는 미니PC 오픈스택 API를 호출하여 실제 사양 목록을 가져옵니다.
func (r *Repository) FetchFlavors() ([]flavors.Flavor, error) {
	// 1. Compute(Nova) 서비스 클라이언트 생성
	// admin.rc에 있던 RegionOne을 명시해줍니다.
	client, err := openstack.NewComputeV2(r.Client, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
	if err != nil {
		return nil, err
	}

	// 2. 상세 사양(vCPU, RAM, Disk 등)이 포함된 Flavor 목록 조회
	allPages, err := flavors.ListDetail(client, nil).AllPages()
	if err != nil {
		return nil, err
	}

	// 3. 페이지 형태의 데이터를 슬라이스([]Flavor)로 추출
	return flavors.ExtractFlavors(allPages)
}

func (r *Repository) FetchInstances() ([]servers.Server, error) {
    client, _ := openstack.NewComputeV2(r.Client, gophercloud.EndpointOpts{Region: "RegionOne"})
    
    // 모든 서버 목록 가져오기
    allPages, _ := servers.List(client, servers.ListOpts{}).AllPages()
    return servers.ExtractServers(allPages)
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