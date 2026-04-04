package access

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"time"
)

// NamespaceDialer dials TCP connections through an OpenStack qrouter network namespace.
type NamespaceDialer struct {
	Namespace string // e.g. "qrouter-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}

// DialVM opens a TCP connection to vmIP:22 via `ip netns exec <ns> nc -w 10 <vmIP> 22`.
// The returned net.Conn wraps the subprocess stdin/stdout as a bidirectional stream.
func (d *NamespaceDialer) DialVM(ctx context.Context, vmIP string) (net.Conn, error) {
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", d.Namespace, "nc", "-w", "10", vmIP, "22")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("namespace dial: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("namespace dial: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("namespace dial: start: %w", err)
	}

	return &nsConn{
		reader:    stdout,
		writer:    stdin,
		cmd:       cmd,
		remoteStr: vmIP + ":22",
	}, nil
}

// nsConn implements net.Conn over a subprocess's stdin/stdout.
type nsConn struct {
	reader    io.ReadCloser
	writer    io.WriteCloser
	cmd       *exec.Cmd
	remoteStr string
}

func (c *nsConn) Read(b []byte) (int, error)  { return c.reader.Read(b) }
func (c *nsConn) Write(b []byte) (int, error) { return c.writer.Write(b) }

func (c *nsConn) Close() error {
	_ = c.writer.Close()
	_ = c.reader.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}

func (c *nsConn) LocalAddr() net.Addr                { return nsAddr("local") }
func (c *nsConn) RemoteAddr() net.Addr               { return nsAddr(c.remoteStr) }
func (c *nsConn) SetDeadline(_ time.Time) error      { return nil }
func (c *nsConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *nsConn) SetWriteDeadline(_ time.Time) error { return nil }

// nsAddr is a minimal net.Addr implementation for the subprocess connection.
type nsAddr string

func (a nsAddr) Network() string { return "tcp" }
func (a nsAddr) String() string  { return string(a) }
