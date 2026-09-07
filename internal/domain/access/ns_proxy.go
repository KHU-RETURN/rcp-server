package access

import (
	"context"
	"net"

	"golang.org/x/net/proxy"
)

type unixForwarder struct {
	path string
}

func (u *unixForwarder) Dial(network, address string) (net.Conn, error) {
	return u.DialContext(context.Background(), network, address)
}

func (u *unixForwarder) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", u.path)
}

func dialViaNSProxy(ctx context.Context, sockPath, address string) (net.Conn, error) {
	forward := &unixForwarder{path: sockPath}
	dialer, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		return nil, err
	}
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, "tcp", address)
	}
	return dialer.Dial("tcp", address)
}
