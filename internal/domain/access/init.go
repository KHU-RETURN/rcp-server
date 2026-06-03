package access

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(provider *gophercloud.ProviderClient, entClient *ent.Client, internalSecret []byte) *Handler {
	osClient := NewClient(provider)
	repo := NewRepository(entClient)
	svc := NewService(osClient, repo)
	h := NewHandler(svc)
	h.InternalSecret = internalSecret
	return h
}

// InitSSH wires the ssh-gateway notify client and SSH callback service.
// auth domain consumes the returned *SSHService via its sshCallbackHandler
// interface (duck-typed).
func InitSSH(notifySockPath string, notifySecret []byte) *SSHService {
	return NewSSHService(NewNotifyClient(notifySockPath, notifySecret))
}
