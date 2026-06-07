package admin

import "time"

type SummaryResponse struct {
	Users        int            `json:"users"`
	Instances    int            `json:"instances"`
	Containers   int            `json:"containers"`
	Keypairs     int            `json:"keypairs"`
	StatusCounts map[string]int `json:"status_counts"`
}

type UserResponse struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	InstanceCount  int       `json:"instance_count"`
	ContainerCount int       `json:"container_count"`
	KeypairCount   int       `json:"keypair_count"`
}

type PaginationResponse struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type PageParams struct {
	Page  int
	Limit int
}

type PaginatedUsersResponse struct {
	Items      []UserResponse     `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

type InstanceResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	OwnerID    string    `json:"owner_id"`
	OwnerEmail string    `json:"owner_email"`
	OwnerName  string    `json:"owner_name"`
	FlavorID   string    `json:"flavor_id"`
	FlavorName string    `json:"flavor_name"`
	ImageID    string    `json:"image_id"`
	FixedIP    string    `json:"fixed_ip"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PaginatedInstancesResponse struct {
	Items      []InstanceResponse `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

type ContainerResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	OpenstackName string    `json:"openstack_name"`
	OwnerID       string    `json:"owner_id"`
	OwnerEmail    string    `json:"owner_email"`
	OwnerName     string    `json:"owner_name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PaginatedContainersResponse struct {
	Items      []ContainerResponse `json:"items"`
	Pagination PaginationResponse  `json:"pagination"`
}

type KeypairResponse struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Fingerprint   string    `json:"fingerprint"`
	SourceType    string    `json:"source_type"`
	InstanceCount int       `json:"instance_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UserResourcesResponse struct {
	User       UserResponse        `json:"user"`
	Instances  []InstanceResponse  `json:"instances"`
	Containers []ContainerResponse `json:"containers"`
	Keypairs   []KeypairResponse   `json:"keypairs"`
}

type SystemResponse struct {
	APIStatus        string    `json:"api_status"`
	OpenStackStatus  string    `json:"openstack_status"`
	SSHGatewayStatus string    `json:"ssh_gateway_status"`
	NSProxyStatus    string    `json:"ns_proxy_status"`
	HTTPProxyStatus  string    `json:"http_proxy_status"`
	StorageStatus    string    `json:"storage_status"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
	Message          string    `json:"message"`
}
