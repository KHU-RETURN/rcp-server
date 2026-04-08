package access

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// handleInteractive는 인터랙티브 모드(모드 1)를 처리합니다.
// VM 목록 메뉴를 표시하고, 선택된 VM으로 Full SSH 릴레이를 수행합니다.
func (s *SSHServer) handleInteractive(channel gossh.Channel, reqs <-chan *gossh.Request) {
	defer channel.Close()

	go handleSessionRequests(reqs)

	vms, err := s.fetchActiveVMs()
	if err != nil {
		fmt.Fprintf(channel, "VM 목록 조회 실패: %v\r\n", err)
		return
	}

	if len(vms) == 0 {
		fmt.Fprint(channel, "접속 가능한 VM이 없습니다.\r\n")
		return
	}

	vm, err := s.runMenuLoop(channel, vms)
	if err != nil {
		fmt.Fprintf(channel, "%v\r\n", err)
		return
	}

	fmt.Fprintf(channel, "\r\n%s (%s) 에 접속 중...\r\n", vm.Name, vm.IP)

	vmConn, err := s.dialVM(vm.IP)
	if err != nil {
		fmt.Fprintf(channel, "VM 연결 실패: %v\r\n", err)
		return
	}
	defer vmConn.Close()

	s.relaySSH(channel, vmConn, vm.IP)
}

// handleDirect는 직접 연결 모드(모드 2)를 처리합니다.
// username+vmname 형식으로 접속 시 raw TCP 릴레이를 수행합니다.
func (s *SSHServer) handleDirect(channel gossh.Channel, email, vmName string) {
	defer channel.Close()

	vm, err := s.findVMByName(vmName)
	if err != nil {
		log.Printf("VM 조회 실패 (user=%s, vm=%s): %v", email, vmName, err)
		return
	}

	vmConn, err := s.dialVM(vm.IP)
	if err != nil {
		log.Printf("VM 연결 실패 (user=%s, vm=%s): %v", email, vmName, err)
		return
	}
	defer vmConn.Close()

	proxyTCP(channel, vmConn)
}

// relaySSH는 RCP 서비스 키로 VM에 SSH 인증 후, 사용자 채널과 VM 채널을 브릿지합니다.
func (s *SSHServer) relaySSH(channel gossh.Channel, vmConn net.Conn, vmIP string) {
	vmSSHConfig := &gossh.ClientConfig{
		User: "ubuntu",
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(s.serviceKey),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	conn, chans, reqs, err := gossh.NewClientConn(vmConn, vmIP+":22", vmSSHConfig)
	if err != nil {
		fmt.Fprintf(channel, "VM SSH 인증 실패: %v\r\n", err)
		return
	}
	defer conn.Close()

	client := gossh.NewClient(conn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(channel, "VM 세션 생성 실패: %v\r\n", err)
		return
	}
	defer session.Close()

	session.Stdin = channel
	session.Stdout = channel
	session.Stderr = channel.Stderr()

	if err := session.RequestPty("xterm", 80, 40, gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		fmt.Fprintf(channel, "PTY 요청 실패: %v\r\n", err)
		return
	}

	if err := session.Shell(); err != nil {
		fmt.Fprintf(channel, "쉘 시작 실패: %v\r\n", err)
		return
	}

	session.Wait()
}

// proxyTCP는 사용자 채널과 VM TCP 연결 사이에서 raw 바이트를 양방향 릴레이합니다.
func proxyTCP(channel gossh.Channel, vmConn io.ReadWriteCloser) {
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(vmConn, channel)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(channel, vmConn)
		done <- struct{}{}
	}()

	<-done
}

// handleSessionRequests는 SSH 세션 요청(pty-req, shell 등)을 처리합니다.
func handleSessionRequests(reqs <-chan *gossh.Request) {
	for req := range reqs {
		switch req.Type {
		case "pty-req", "shell":
			req.Reply(true, nil)
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// runMenuLoop는 사용자에게 VM 목록을 표시하고 선택을 받습니다.
func (s *SSHServer) runMenuLoop(channel gossh.Channel, vms []VMInfo) (*VMInfo, error) {
	page := 0
	totalPages := (len(vms) + s.menuPageSize - 1) / s.menuPageSize

	for {
		s.renderMenu(channel, vms, page, totalPages)

		input, err := readLine(channel)
		if err != nil {
			return nil, fmt.Errorf("입력 읽기 실패: %w", err)
		}
		input = strings.TrimSpace(input)

		switch input {
		case "q":
			return nil, fmt.Errorf("사용자가 종료함")
		case "n":
			if page < totalPages-1 {
				page++
			}
		case "p":
			if page > 0 {
				page--
			}
		default:
			num, err := strconv.Atoi(input)
			if err != nil {
				fmt.Fprint(channel, "잘못된 입력입니다.\r\n")
				continue
			}
			idx := num - 1
			if idx < 0 || idx >= len(vms) {
				fmt.Fprint(channel, "범위를 벗어난 번호입니다.\r\n")
				continue
			}
			return &vms[idx], nil
		}
	}
}

// renderMenu는 VM 목록 페이지를 터미널에 렌더링합니다.
func (s *SSHServer) renderMenu(w io.Writer, vms []VMInfo, page, totalPages int) {
	start := page * s.menuPageSize
	end := start + s.menuPageSize
	if end > len(vms) {
		end = len(vms)
	}

	fmt.Fprintf(w, "\r\n")
	fmt.Fprintf(w, "┌─────────────────────────────────────────────┐\r\n")
	fmt.Fprintf(w, "│  접속 가능한 VM 목록  (%d-%d / %d)%s│\r\n",
		start+1, end, len(vms), strings.Repeat(" ", max(0, 14-len(fmt.Sprintf("%d-%d / %d", start+1, end, len(vms))))))
	fmt.Fprintf(w, "├─────────────────────────────────────────────┤\r\n")

	for i := start; i < end; i++ {
		vm := vms[i]
		fmt.Fprintf(w, "│  %2d. %-20s %-15s │\r\n", i+1, vm.Name, vm.IP)
	}

	fmt.Fprintf(w, "├─────────────────────────────────────────────┤\r\n")
	fmt.Fprintf(w, "│  번호 입력 / n: 다음 / p: 이전 / q: 종료    │\r\n")
	fmt.Fprintf(w, "│  > ")
}

// readLine은 SSH 채널에서 한 줄을 읽습니다.
func readLine(r io.Reader) (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if err != nil {
			return string(buf), err
		}
		if n == 0 {
			continue
		}
		if b[0] == '\r' || b[0] == '\n' {
			return string(buf), nil
		}
		buf = append(buf, b[0])
	}
}
