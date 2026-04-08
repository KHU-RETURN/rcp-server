package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/config"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	gossh "golang.org/x/crypto/ssh"
)

// userVerifier는 이메일로 RCP 등록 유저를 확인합니다.
type userVerifier interface {
	verifyUser(ctx context.Context, email string) (bool, error)
}

// userVerifierFunc는 함수를 userVerifier 인터페이스로 적응합니다.
type userVerifierFunc func(ctx context.Context, email string) (bool, error)

func (f userVerifierFunc) verifyUser(ctx context.Context, email string) (bool, error) {
	return f(ctx, email)
}

// SSHServer는 SSH 릴레이 서버입니다.
type SSHServer struct {
	config       *gossh.ServerConfig
	listenAddr   string
	verifier     userVerifier
	provider     *gophercloud.ProviderClient
	namespace    string
	serviceKey   gossh.Signer
	menuPageSize int
}

// serverAddress는 OpenStack 서버 주소 정보입니다.
type serverAddress struct {
	Address string `json:"addr"`
	Type    string `json:"OS-EXT-IPS:type"`
}

// nsConn은 subprocess의 stdin/stdout을 net.Conn 인터페이스로 래핑합니다.
type nsConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

type dummyAddr struct{}

// NewSSHServer는 SSH 릴레이 서버를 생성합니다.
func NewSSHServer(cfg *config.SSHConfig, authRepo *auth.Repository, p *gophercloud.ProviderClient) (*SSHServer, error) {
	hostKeyBytes, err := os.ReadFile(cfg.HostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read host key: %w", err)
	}
	hostKey, err := gossh.ParsePrivateKey(hostKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	caBytes, err := os.ReadFile(cfg.CAPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA public key: %w", err)
	}
	caKey, _, _, _, err := gossh.ParseAuthorizedKey(caBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA public key: %w", err)
	}

	var serviceKey gossh.Signer
	if cfg.ServiceKeyPath != "" {
		skBytes, err := os.ReadFile(cfg.ServiceKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read service key: %w", err)
		}
		serviceKey, err = gossh.ParsePrivateKey(skBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse service key: %w", err)
		}
	}

	pageSize := cfg.MenuPageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	s := &SSHServer{
		listenAddr: ":" + cfg.ListenPort,
		verifier: userVerifierFunc(func(ctx context.Context, email string) (bool, error) {
			return authRepo.ExistsByEmail(ctx, email)
		}),
		provider:     p,
		namespace:    cfg.Namespace,
		serviceKey:   serviceKey,
		menuPageSize: pageSize,
	}

	sshConfig := &gossh.ServerConfig{
		PublicKeyCallback: s.certCallback(caKey),
	}
	sshConfig.AddHostKey(hostKey)
	s.config = sshConfig

	return s, nil
}

// ListenAndServe는 SSH 서버를 시작합니다.
func (s *SSHServer) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}
	defer listener.Close()

	log.Printf("SSH 릴레이 서버 시작: %s", s.listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("SSH accept 실패: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// certCallback은 CF CA 공개키로 서명된 인증서를 검증하는 콜백을 반환합니다.
func (s *SSHServer) certCallback(caKey gossh.PublicKey) func(gossh.ConnMetadata, gossh.PublicKey) (*gossh.Permissions, error) {
	certChecker := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), caKey.Marshal())
		},
	}

	return func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
		cert, ok := key.(*gossh.Certificate)
		if !ok {
			return nil, errors.New("only certificate authentication is supported")
		}

		perm, err := certChecker.Authenticate(conn, key)
		if err != nil {
			return nil, fmt.Errorf("certificate verification failed: %w", err)
		}

		if len(cert.ValidPrincipals) == 0 {
			return nil, errors.New("certificate has no principals")
		}
		email := cert.ValidPrincipals[0]

		exists, err := s.verifier.verifyUser(context.Background(), email)
		if err != nil {
			return nil, fmt.Errorf("user verification failed: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("user %s is not registered", email)
		}

		if perm.Extensions == nil {
			perm.Extensions = make(map[string]string)
		}
		perm.Extensions["email"] = email

		return perm, nil
	}
}

// handleConnection은 SSH 핸드셰이크 후 채널을 수락하고 모드를 라우팅합니다.
func (s *SSHServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	sshConn, chans, reqs, err := gossh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("SSH 핸드셰이크 실패: %v", err)
		return
	}
	defer sshConn.Close()

	go gossh.DiscardRequests(reqs)

	email := sshConn.Permissions.Extensions["email"]
	username := sshConn.User()
	_, vmName := parseUsername(username)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(gossh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChan.Accept()
		if err != nil {
			log.Printf("채널 수락 실패: %v", err)
			continue
		}

		if vmName != "" {
			go s.handleDirect(channel, email, vmName)
			go gossh.DiscardRequests(requests)
		} else {
			go s.handleInteractive(channel, requests)
		}
	}
}

// fetchActiveVMs는 OpenStack에서 ACTIVE 상태의 VM 목록을 조회합니다.
func (s *SSHServer) fetchActiveVMs() ([]VMInfo, error) {
	sc, err := goopenstack.NewComputeV2(s.provider, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	allPages, err := servers.List(sc, servers.ListOpts{Status: "ACTIVE"}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	raw, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract servers: %w", err)
	}

	var result []VMInfo
	for _, srv := range raw {
		ip := extractFixedIP(srv.Addresses)
		if ip == "" {
			continue
		}
		result = append(result, VMInfo{
			ID:     srv.ID,
			Name:   srv.Name,
			Status: srv.Status,
			IP:     ip,
		})
	}
	return result, nil
}

// findVMByName은 이름으로 VM을 검색합니다.
func (s *SSHServer) findVMByName(name string) (*VMInfo, error) {
	vms, err := s.fetchActiveVMs()
	if err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.Name == name {
			return &vm, nil
		}
	}
	return nil, fmt.Errorf("VM %q not found", name)
}

// dialVM은 qrouter namespace를 경유하여 VM의 SSH 포트에 TCP 연결합니다.
func (s *SSHServer) dialVM(vmIP string) (net.Conn, error) {
	cmd := exec.Command("ip", "netns", "exec", s.namespace, "nc", "-w", "10", vmIP, "22")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start netns command: %w", err)
	}

	return &nsConn{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

func (c *nsConn) Read(b []byte) (int, error)  { return c.stdout.Read(b) }
func (c *nsConn) Write(b []byte) (int, error) { return c.stdin.Write(b) }

func (c *nsConn) Close() error {
	c.stdin.Close()
	c.stdout.Close()
	return c.cmd.Process.Kill()
}

func (c *nsConn) LocalAddr() net.Addr                { return dummyAddr{} }
func (c *nsConn) RemoteAddr() net.Addr               { return dummyAddr{} }
func (c *nsConn) SetDeadline(t time.Time) error      { return nil }
func (c *nsConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *nsConn) SetWriteDeadline(t time.Time) error { return nil }

func (dummyAddr) Network() string { return "nsConn" }
func (dummyAddr) String() string  { return "nsConn" }

// parseUsername은 SSH username에서 모드와 VM 이름을 파싱합니다.
// "user" → ("user", ""), "user+vmname" → ("user", "vmname")
func parseUsername(username string) (user, vmName string) {
	parts := strings.SplitN(username, "+", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return username, ""
}

// extractFixedIP는 OpenStack Addresses에서 사설(fixed) IP를 추출합니다.
func extractFixedIP(addresses map[string]any) string {
	for _, addrs := range addresses {
		var parsed []serverAddress
		raw, err := json.Marshal(addrs)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			continue
		}
		for _, a := range parsed {
			if a.Type == "fixed" {
				return a.Address
			}
		}
	}
	return ""
}
