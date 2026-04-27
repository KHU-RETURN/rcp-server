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

// makeRequestWithCmd builds a *socks5.Request with a command and DestAddr,
// for testing RuleSet command gating.
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

	// IP is inside the allowlist, but ASSOCIATE must be rejected regardless.
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

	// IP is inside the allowlist, but BIND must be rejected regardless.
	_, ok := r.Allow(context.Background(), makeRequestWithCmd(statute.CommandBind, net.ParseIP("192.168.1.10")))
	if ok {
		t.Error("expected deny for BIND command even with allowed IP")
	}
}

// TestIpv4OnlyResolver_ResolvesLocalhost uses "localhost" rather than a literal
// IP. The pure-Go resolver (CGO_ENABLED=0) can fail for literal IPs because it
// tries to resolve them as hostnames; "localhost" is handled by /etc/hosts on
// both Linux and macOS and always returns an IPv4 address.
func TestIpv4OnlyResolver_ResolvesLocalhost(t *testing.T) {
	r := ipv4OnlyResolver{}
	_, ip, err := r.Resolve(context.Background(), "localhost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Don't assert exact IP — localhost may be 127.0.0.1 or 127.0.1.1
	// depending on /etc/hosts.
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

// TODO: the "no IPv4 in resolver results" branch in ipv4OnlyResolver.Resolve is
// unreachable from these tests because net.DefaultResolver isn't injectable.
// If the resolver becomes injectable later (e.g. for testability), add a test
// using a fake that returns only IPv6 addresses.

// ---------- integration tests: command gating via live SOCKS5 server ----------
//
// These tests spin up a real NewSOCKS5Server and send raw SOCKS5 bytes through
// net.Pipe to verify that the server rejects BIND/ASSOCIATE at the protocol
// level and accepts CONNECT to an allowed destination.

// socks5Handshake sends the SOCKS5 NoAuth greeting and reads the method
// selection reply. Returns an error if the server did not select NoAuth (0x00).
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

// socks5CommandReply sends a SOCKS5 command request to 127.0.0.1:<port> using
// the given command byte and returns the server's reply code (byte [1] of the
// response).
func socks5CommandReply(conn net.Conn, cmd byte, ip [4]byte, port uint16) (byte, error) {
	req := []byte{
		0x05, cmd, 0x00,
		0x01, ip[0], ip[1], ip[2], ip[3],
		byte(port >> 8), byte(port),
	}
	if _, err := conn.Write(req); err != nil {
		return 0, err
	}
	// SOCKS5 reply: VER RSP RSV ATYP BND.ADDR(4) BND.PORT(2) = 10 bytes for IPv4
	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, err
	}
	return resp[1], nil
}

// newTestServer creates a NewSOCKS5Server configured with 127.0.0.0/8 allowlist
// and a short dial timeout (used by integration tests).
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

// runServerConn spins up the server on one end of a net.Pipe connection.
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
	// Send ASSOCIATE for 127.0.0.1:0 — IP is in allowlist but command must be rejected.
	reply, err := socks5CommandReply(client, statute.CommandAssociate, [4]byte{127, 0, 0, 1}, 0)
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	// Expect RepRuleFailure (0x02) — cidrRuleSet.Allow rejects non-CONNECT commands.
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
	// Send BIND for 127.0.0.1:0 — IP is in allowlist but command must be rejected.
	reply, err := socks5CommandReply(client, statute.CommandBind, [4]byte{127, 0, 0, 1}, 0)
	if err != nil {
		t.Fatalf("read reply failed: %v", err)
	}
	if reply == statute.RepSuccess {
		t.Errorf("BIND was allowed (reply=0x%02x); expected non-success", reply)
	}
}

func TestNewSOCKS5Server_AllowsConnect(t *testing.T) {
	// Start a small in-process echo server to CONNECT to.
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

	// Verify the tunnel works: send a payload and read it back.
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
