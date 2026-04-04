package access

import (
	"context"
	"io"
	"log"

	gossh "golang.org/x/crypto/ssh"
)

// handleInteractiveRelay dials the target VM via the namespace and establishes
// a full SSH client connection to it using the RCP service key.
// It then bridges the user's SSH channel to the VM's SSH channel,
// forwarding PTY, window-change, and stdin/stdout/stderr.
func (h *ConnectionHandler) handleInteractiveRelay(
	clientChan gossh.Channel,
	clientReqs <-chan *gossh.Request,
	vmIP string,
) {
	ctx := context.Background()

	nsConn, err := h.svc.DialVM(ctx, vmIP)
	if err != nil {
		log.Printf("relay: dial vm %s: %v", vmIP, err)
		sendSSHExitStatus(clientChan, 1)
		return
	}
	defer func() { _ = nsConn.Close() }()

	sshClientConn, chans, reqs, err := gossh.NewClientConn(nsConn, vmIP+":22", &gossh.ClientConfig{
		User: vmDefaultUser,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(h.svc.GetServiceKey()),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // internal network, no TOFU needed
	})
	if err != nil {
		log.Printf("relay: ssh client conn to %s: %v", vmIP, err)
		sendSSHExitStatus(clientChan, 1)
		return
	}
	defer func() { _ = sshClientConn.Close() }()

	client := gossh.NewClient(sshClientConn, chans, reqs)
	defer func() { _ = client.Close() }()

	vmSession, err := client.NewSession()
	if err != nil {
		log.Printf("relay: new session on %s: %v", vmIP, err)
		sendSSHExitStatus(clientChan, 1)
		return
	}
	defer func() { _ = vmSession.Close() }()

	vmStdin, err := vmSession.StdinPipe()
	if err != nil {
		log.Printf("relay: vm stdin pipe: %v", err)
		return
	}
	vmStdout, err := vmSession.StdoutPipe()
	if err != nil {
		log.Printf("relay: vm stdout pipe: %v", err)
		return
	}
	vmStderr, err := vmSession.StderrPipe()
	if err != nil {
		log.Printf("relay: vm stderr pipe: %v", err)
		return
	}

	go func() { _, _ = io.Copy(vmStdin, clientChan) }()
	go func() { _, _ = io.Copy(clientChan, vmStdout) }()
	go func() { _, _ = io.Copy(clientChan.Stderr(), vmStderr) }()

	if clientReqs != nil {
		go forwardSSHSessionRequests(clientReqs, vmSession)
	} else {
		// No requests to forward (called from menu after PTY is already set up)
		if err := vmSession.Shell(); err != nil {
			log.Printf("relay: start shell: %v", err)
		}
	}

	// Wait for VM session to finish
	if err := vmSession.Wait(); err != nil {
		log.Printf("relay: vm session ended: %v", err)
	}
	sendSSHExitStatus(clientChan, 0)
}

// forwardSSHSessionRequests proxies SSH session requests from the client channel
// to the VM session (PTY, shell, window-change, env, exec).
func forwardSSHSessionRequests(reqs <-chan *gossh.Request, session *gossh.Session) {
	for req := range reqs {
		switch req.Type {
		case requestPTY:
			var ptyReq struct {
				Term     string
				Width    uint32
				Height   uint32
				PixWidth uint32
				PixHigh  uint32
				Modes    string
			}
			if err := gossh.Unmarshal(req.Payload, &ptyReq); err == nil {
				_ = session.RequestPty(ptyReq.Term, int(ptyReq.Height), int(ptyReq.Width), gossh.TerminalModes{})
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

		case requestWindowChange:
			var winch struct {
				Width    uint32
				Height   uint32
				PixWidth uint32
				PixHigh  uint32
			}
			if err := gossh.Unmarshal(req.Payload, &winch); err == nil {
				_ = session.WindowChange(int(winch.Height), int(winch.Width))
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

		case requestShell:
			if err := session.Shell(); err != nil {
				log.Printf("relay: start shell: %v", err)
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

		case requestExec:
			var execReq struct{ Command string }
			if err := gossh.Unmarshal(req.Payload, &execReq); err == nil {
				if err := session.Start(execReq.Command); err != nil {
					log.Printf("relay: exec %q: %v", execReq.Command, err)
				}
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}

		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// sendSSHExitStatus sends an exit-status request to the client channel.
func sendSSHExitStatus(ch gossh.Channel, code uint32) {
	payload := gossh.Marshal(struct{ Code uint32 }{Code: code})
	_, _ = ch.SendRequest(requestExitStatus, false, payload)
	_ = ch.CloseWrite()

	// Drain remaining input from client
	buf := make([]byte, 1024)
	for {
		n, err := ch.Read(buf)
		if n == 0 || err != nil {
			break
		}
	}
	_ = ch.Close()
}
