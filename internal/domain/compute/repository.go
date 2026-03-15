package compute

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
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