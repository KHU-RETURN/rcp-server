package compute

import "github.com/gophercloud/gophercloud"

// Init wires the compute domain.
// registrar receives VM create/delete events and records them in the SSH relay's user_vms table.
// Pass nil if VM ownership tracking is not needed.
func Init(p *gophercloud.ProviderClient, registrar VMRegistrar) *Handler {
	repo := NewRepository(p)
	svc := NewService(repo, registrar)
	return NewHandler(svc)
}
