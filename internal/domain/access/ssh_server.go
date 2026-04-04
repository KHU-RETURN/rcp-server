package access

import (
	"fmt"
	"log"
	"net"

	gossh "golang.org/x/crypto/ssh"
)

// SSHServer listens for incoming SSH connections and dispatches them to the handler.
type SSHServer struct {
	config     *gossh.ServerConfig
	listenAddr string
	handler    *ConnectionHandler
}

// NewSSHServer creates an SSHServer ready to listen on cfg.ListenAddr.
func NewSSHServer(cfg SSHConfig, serverConfig *gossh.ServerConfig, handler *ConnectionHandler) *SSHServer {
	return &SSHServer{
		config:     serverConfig,
		listenAddr: cfg.ListenAddr,
		handler:    handler,
	}
}

// Addr returns the configured listen address.
func (s *SSHServer) Addr() string {
	return s.listenAddr
}

// ListenAndServe starts the SSH server. Blocks until the listener fails.
func (s *SSHServer) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("ssh server listen %s: %w", s.listenAddr, err)
	}
	defer func() { _ = listener.Close() }()

	log.Printf("SSH relay server listening on %s", s.listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("ssh server accept: %w", err)
		}
		go s.handleTCPConn(conn)
	}
}

// handleTCPConn performs the SSH handshake on a raw TCP connection and
// dispatches authenticated connections to the handler.
func (s *SSHServer) handleTCPConn(conn net.Conn) {
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("SSH handshake failed from %s: %v", conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}
	defer func() { _ = sshConn.Close() }()

	email, ok := sshConn.Permissions.Extensions["email"]
	if !ok {
		log.Printf("SSH conn missing email extension from %s", conn.RemoteAddr())
		return
	}

	log.Printf("SSH authenticated: %s from %s", email, conn.RemoteAddr())
	s.handler.HandleConnection(sshConn, chans, reqs, email)
}
