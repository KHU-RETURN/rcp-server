package access

import (
	"context"
	"fmt"
	"log"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// handleInteractiveSession implements Mode 1: shows a paginated VM selection menu
// over a PTY session, then relays to the chosen VM.
func (h *ConnectionHandler) handleInteractiveSession(
	_ *gossh.ServerConn,
	chans <-chan gossh.NewChannel,
	email string,
) {
	ctx := context.Background()

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(gossh.UnknownChannelType, "only session channels are supported")
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			log.Printf("menu: accept channel: %v", err)
			return
		}

		go h.runMenuSession(ctx, ch, reqs, email)
		return // one session per connection
	}
}

// runMenuSession handles PTY setup, renders the VM menu, reads user input,
// and dispatches to the selected VM.
func (h *ConnectionHandler) runMenuSession(
	ctx context.Context,
	ch gossh.Channel,
	reqs <-chan *gossh.Request,
	email string,
) {
	defer ch.Close()

	var hasPTY bool
	for req := range reqs {
		switch req.Type {
		case "pty-req":
			hasPTY = true
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "shell":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			goto showMenu
		case "exec":
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}

showMenu:
	if !hasPTY {
		sshWriteString(ch, "Error: a terminal (PTY) is required for the interactive menu.\r\n")
		return
	}

	vms, err := h.svc.ListUserVMs(ctx, email)
	if err != nil {
		log.Printf("menu: list vms for %s: %v", email, err)
		sshWriteString(ch, "Error: could not load VM list.\r\n")
		return
	}

	if len(vms) == 0 {
		sshWriteString(ch, "No VMs available.\r\n")
		return
	}

	pageSize := h.svc.MenuPageSize()
	page := 0

	for {
		renderSSHMenu(ch, vms, page, pageSize)

		input, err := sshReadLine(ch)
		if err != nil {
			return
		}
		input = strings.TrimSpace(input)

		switch input {
		case "q", "Q":
			sshWriteString(ch, "Goodbye.\r\n")
			return
		case "n", "N":
			if (page+1)*pageSize < len(vms) {
				page++
			}
		case "p", "P":
			if page > 0 {
				page--
			}
		default:
			n := 0
			if _, err := fmt.Sscanf(input, "%d", &n); err != nil || n < 1 {
				sshWriteString(ch, "Invalid input. Enter a number, n, p, or q.\r\n")
				continue
			}
			idx := page*pageSize + (n - 1)
			if idx >= len(vms) {
				sshWriteString(ch, fmt.Sprintf("No VM #%d on this page.\r\n", n))
				continue
			}
			selected := vms[idx]
			sshWriteString(ch, fmt.Sprintf("\r\nConnecting to %s (%s)...\r\n", selected.VMName, selected.FixedIP))

			h.handleInteractiveRelay(ch, nil, selected.FixedIP)
			return
		}
	}
}

// renderSSHMenu writes the paginated VM list to the SSH channel.
func renderSSHMenu(ch gossh.Channel, vms []UserVM, page, pageSize int) {
	total := len(vms)
	start := page * pageSize
	end := min(start+pageSize, total)

	header := fmt.Sprintf("%d-%d / %d", start+1, end, total)
	pad := max(13-len(header), 0)

	var b strings.Builder
	b.WriteString("\r\n")
	b.WriteString("┌─────────────────────────────────────────────┐\r\n")
	b.WriteString(fmt.Sprintf("│  접속 가능한 VM 목록  (%s)%s│\r\n", header, strings.Repeat(" ", pad)))
	b.WriteString("├─────────────────────────────────────────────┤\r\n")

	for i := start; i < end; i++ {
		vm := vms[i]
		num := i - start + 1
		line := fmt.Sprintf("│  %2d. %-16s %-15s│\r\n", num, vm.VMName, vm.FixedIP)
		b.WriteString(line)
	}

	b.WriteString("├─────────────────────────────────────────────┤\r\n")
	b.WriteString("│  번호 입력 / n: 다음 / p: 이전 / q: 종료   │\r\n")
	b.WriteString("│  > ")
	sshWriteString(ch, b.String())
}

// sshReadLine reads a single line of user input from the SSH channel.
// Handles backspace and echoes characters back to the terminal.
func sshReadLine(ch gossh.Channel) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := ch.Read(buf)
		if err != nil || n == 0 {
			return "", err
		}
		c := buf[0]
		switch {
		case c == '\r' || c == '\n':
			sshWriteString(ch, "\r\n")
			return string(line), nil
		case c == 127 || c == '\b': // backspace
			if len(line) > 0 {
				line = line[:len(line)-1]
				sshWriteString(ch, "\b \b")
			}
		default:
			line = append(line, c)
			sshWriteString(ch, string([]byte{c})) // echo
		}
	}
}

// sshWriteString writes s to the SSH channel, ignoring errors.
func sshWriteString(ch gossh.Channel, s string) {
	_, _ = ch.Write([]byte(s))
}
