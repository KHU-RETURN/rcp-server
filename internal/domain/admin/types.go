package admin

import "time"

type SummaryResponse struct {
	Users        int            `json:"users"`
	Instances    int            `json:"instances"`
	Containers   int            `json:"containers"`
	Apps         int            `json:"apps"`
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
	AppCount       int       `json:"app_count"`
	KeypairCount   int       `json:"keypair_count"`
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
	AppHost    string    `json:"app_host"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SystemResponse struct {
	APIStatus        string    `json:"api_status"`
	OpenStackStatus  string    `json:"openstack_status"`
	SSHGatewayStatus string    `json:"ssh_gateway_status"`
	StorageStatus    string    `json:"storage_status"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
	Message          string    `json:"message"`
}
