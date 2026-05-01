package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	socks5 "github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

// makeRequestWithCmd는 RuleSet 커맨드 게이팅 검사용 *socks5.Request를 만든다.
func makeRequestWithCmd(cmd byte, ip net.IP) *socks5.Request {
	req := &socks5.Request{
		DestAddr: &statute.AddrSpec{IP: ip},
	}
	req.Command = cmd
	return req
}

func TestCidrRuleSet_AllowsIPv4InAllowlist(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandConnect, net.ParseIP("192.168.1.10")))
	if !ok {
		t.Error("expected allow for 192.168.1.10 (inside 192.168.0.0/16)")
	}
}

func TestCidrRuleSet_DeniesIPv4OutsideAllowlist(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandConnect, net.ParseIP("8.8.8.8")))
	if ok {
		t.Error("expected deny for 8.8.8.8 (outside 192.168.0.0/16)")
	}
}

func TestCidrRuleSet_DeniesIPv6Always(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandConnect, net.ParseIP("2001:db8::1")))
	if ok {
		t.Error("expected deny for IPv6 destination (defense-in-depth)")
	}
}

func TestCidrRuleSet_DeniesNilIP(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandConnect, nil))
	if ok {
		t.Error("expected deny for nil IP")
	}
}

func TestCidrRuleSet_DeniesAssociateCommand(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	// IP는 allowlist 안이지만 ASSOCIATE는 무조건 거부돼야 함.
	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandAssociate, net.ParseIP("192.168.1.10")))
	if ok {
		t.Error("expected deny for UDP ASSOCIATE command even with allowed IP")
	}
}

func TestCidrRuleSet_DeniesBindCommand(t *testing.T) {
	al, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	r := &cidrRuleSet{allow: al}

	// IP는 allowlist 안이지만 BIND는 무조건 거부돼야 함.
	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandBind, net.ParseIP("192.168.1.10")))
	if ok {
		t.Error("expected deny for BIND command even with allowed IP")
	}
}

// pure-Go resolver(CGO_ENABLED=0)는 IP literal을 hostname으로 해석하려다 실패할
// 수 있어 "localhost" 사용 — Linux/macOS 모두 /etc/hosts에서 IPv4로 해석됨.
func TestIpv4OnlyResolver_ResolvesLocalhost(t *testing.T) {
	r := ipv4OnlyResolver{}
	_, ip, err := r.Resolve(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// /etc/hosts에 따라 127.0.0.1 또는 127.0.1.1일 수 있어 정확한 IP는 검사 안 함.
	if ip.To4() == nil {
		t.Errorf("got %v, want IPv4", ip)
	}
}

func TestIpv4OnlyResolver_RejectsIPv6Literal(t *testing.T) {
	r := ipv4OnlyResolver{}
	_, _, err := r.Resolve(context.Background(), "::1")
	if err == nil {
		t.Error("expected error for IPv6-only host, got nil")
	}
}

// TODO: ipv4OnlyResolver.Resolve의 "결과에 IPv4 없음" 분기는 net.DefaultResolver를
// 주입할 수 없어 현재 테스트로 못 닿음. 주입 가능해지면 IPv6만 반환하는 fake로 추가.

// ---------- integration tests: 실제 SOCKS5 서버에 raw 바이트로 BIND/ASSOCIATE/CONNECT 검증 ----------

// socks5Handshake는 NoAuth greeting을 보내고 method selection 응답을 읽는다.
// 서버가 NoAuth(0x00)를 선택하지 않으면 에러.
func socks5Handshake(conn net.Conn) error {
	// ClientGreeting: VER=5, NMETHODS=1, METHOD=0x00 (NoAuth)
	_, err := conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// socks5CommandReply는 127.0.0.1:<port>에 대한 커맨드 요청을 보내고 reply byte를 반환.
func socks5CommandReply(conn net.Conn, cmd byte, ip [4]byte, port uint16) (byte, error) {
	req := []byte{
		0x05, cmd, 0x00,
		0x01, ip[0], ip[1], ip[2], ip[3],
		byte(port >> 8), byte(port),
	}
	if _, err := conn.Write(req); err != nil {
		return 0, err
	}
	// SOCKS5 reply: VER RSP RSV ATYP BND.ADDR(4) BND.PORT(2) = IPv4의 경우 10 bytes.
	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, err
	}
	return resp[1], nil
}

// newTestServer는 127.0.0.0/8 allowlist + 짧은 dial timeout으로 구성된 SOCKS5 서버를 만든다.
func newTestServer(t *testing.T) *socks5.Server {
	t.Helper()
	al, err := ParseCIDRs("127.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	cfg := &Config{
		AllowList:   al,
		DialTimeout: 2 * time.Second,
	}
	return NewSOCKS5Server(cfg, slog.Default())
}

func runServerConn(srv *socks5.Server, srvConn net.Conn) {
	_ = srv.ServeConn(srvConn)
}

func TestNewSOCKS5Server_RejectsAssociate(t *testing.T) {
	srv := newTestServer(t)
	client, server := net.Pipe()
	defer client.Close()
	go runServerConn(srv, server)

	if err := socks5Handshake(client); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	// IP는 allowlist 안이지만 ASSOCIATE는 거부돼야 함.
	reply, err := socks5CommandReply(client, statute.CommandAssociate, [4]byte{127, 0, 0, 1}, 0)
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	if reply == statute.RepSuccess {
		t.Errorf("ASSOCIATE was allowed (reply=0x%02x); expected non-success", reply)
	}
}

func TestNewSOCKS5Server_RejectsBind(t *testing.T) {
	srv := newTestServer(t)
	client, server := net.Pipe()
	defer client.Close()
	go runServerConn(srv, server)

	if err := socks5Handshake(client); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	// IP는 allowlist 안이지만 BIND는 거부돼야 함.
	reply, err := socks5CommandReply(client, statute.CommandBind, [4]byte{127, 0, 0, 1}, 0)
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	if reply == statute.RepSuccess {
		t.Errorf("BIND was allowed (reply=0x%02x); expected non-success", reply)
	}
}

func TestNewSOCKS5Server_AllowsConnect(t *testing.T) {
	// CONNECT 대상으로 in-process echo 서버 기동.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	defer echo.Close()
	go func() {
		conn, err := echo.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn) //nolint:errcheck
	}()

	echoPort := echo.Addr().(*net.TCPAddr).Port

	srv := newTestServer(t)
	client, server := net.Pipe()
	defer client.Close()
	go runServerConn(srv, server)

	if err := socks5Handshake(client); err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	reply, err := socks5CommandReply(client, statute.CommandConnect, [4]byte{127, 0, 0, 1}, uint16(echoPort))
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	if reply != statute.RepSuccess {
		t.Fatalf("CONNECT denied (reply=0x%02x); expected RepSuccess", reply)
	}

	// 터널 동작 확인: payload 보내고 그대로 받기.
	payload := []byte("hello-ns-proxy")
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Errorf("echo mismatch: got %q, want %q", buf, payload)
	}
}
