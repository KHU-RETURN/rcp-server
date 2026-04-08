package access

import (
	"log"

	"github.com/KHU-RETURN/rcp-server/internal/config"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gophercloud/gophercloud"
)

func Init(p *gophercloud.ProviderClient) *Handler {
	client := NewClient(p)
	svc := NewService(client)
	return NewHandler(svc)
}

// InitSSH는 SSH 릴레이 서버를 초기화합니다.
// cfg가 nil이면 nil을 반환합니다.
func InitSSH(cfg *config.SSHConfig, authRepo *auth.Repository, p *gophercloud.ProviderClient) *SSHServer {
	if cfg == nil {
		return nil
	}
	s, err := NewSSHServer(cfg, authRepo, p)
	if err != nil {
		log.Printf("SSH 서버 초기화 실패 (건너뜀): %v", err)
		return nil
	}
	return s
}
