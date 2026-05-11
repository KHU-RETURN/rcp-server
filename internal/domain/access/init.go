package access

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(provider *gophercloud.ProviderClient, entClient *ent.Client) *Handler {
	osClient := NewClient(provider)
	repo := NewRepository(entClient)
	svc := NewService(osClient, repo)
	return NewHandler(svc)
}

// InitSSH wires the ssh-gateway notify client and SSH callback service.
// auth domain consumes the returned *SSHService via its sshCallbackHandler
// interface (duck-typed).
func InitSSH(notifySockPath string, notifySecret []byte) *SSHService {
	return NewSSHService(NewNotifyClient(notifySockPath, notifySecret))
}
