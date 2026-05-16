package access

import (
	"net"

	"golang.org/x/net/proxy"
)

type unixForwarder struct {
	path string
}

func (u *unixForwarder) Dial(_, _ string) (net.Conn, error) {
	return net.Dial("unix", u.path)
}

func dialViaNSProxy(sockPath, address string) (net.Conn, error) {
	forward := &unixForwarder{path: sockPath}
	dialer, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		return nil, err
	}
	return dialer.Dial("tcp", address)
}
