package access

import (
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// ConnectionHandler routes authenticated SSH connections to the correct relay mode.
type ConnectionHandler struct {
	svc *SSHService
}

// NewConnectionHandler creates a ConnectionHandler backed by the given service.
func NewConnectionHandler(svc *SSHService) *ConnectionHandler {
	return &ConnectionHandler{svc: svc}
}

// HandleConnection dispatches a connection to interactive menu (mode 1) or
// direct TCP relay (mode 2) based on the parsed username.
//
//   - "user"        → interactive VM selection menu
//   - "user+vmname" → direct relay to named VM
func (h *ConnectionHandler) HandleConnection(
	conn *gossh.ServerConn,
	chans <-chan gossh.NewChannel,
	reqs <-chan *gossh.Request,
	email string,
) {
	// Drain global requests (keepalive, etc.)
	go gossh.DiscardRequests(reqs)

	_, vmName := parseSSHUsername(conn.User())
	if vmName == "" {
		h.handleInteractiveSession(conn, chans, email)
	} else {
		h.handleDirectRelay(conn, chans, email, vmName)
	}
}

// parseSSHUsername splits "user+vmname" into ("user", "vmname").
// If no "+" is present, vmName is empty string (menu mode).
func parseSSHUsername(raw string) (user, vmName string) {
	parts := strings.SplitN(raw, "+", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}
