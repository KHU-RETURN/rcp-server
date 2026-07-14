package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

// socks5HandshakeTimeout bounds the SOCKS5 CONNECT handshake when the caller's
// context carries no deadline of its own.
const socks5HandshakeTimeout = 15 * time.Second

type unixForwarder struct {
	path string
}

func (u *unixForwarder) Dial(network, address string) (net.Conn, error) {
	return u.DialContext(context.Background(), network, address)
}

// DialContext dials the unix socket and holds a deadline over the connection
// so the SOCKS5 handshake that immediately follows (performed by the
// golang.org/x/net/proxy library right after this returns) can't stall
// forever. The caller clears the deadline once the handshake succeeds.
func (u *unixForwarder) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	conn, err := net.Dial("unix", u.path)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(socks5HandshakeTimeout)
	}
	_ = conn.SetDeadline(deadline)
	return conn, nil
}

func transportViaNSProxy(sockPath string) (*http.Transport, error) {
	forward := &unixForwarder{path: sockPath}
	dialer, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		return nil, err
	}
	ctxDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("socks5 dialer missing context support")
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, err := ctxDialer.DialContext(ctx, network, address)
				ch <- result{conn: conn, err: err}
			}()
			select {
			case <-ctx.Done():
				// The dial goroutine may still complete; make sure an
				// abandoned successful dial doesn't leak its connection.
				go func() {
					if res := <-ch; res.conn != nil {
						_ = res.conn.Close()
					}
				}()
				return nil, ctx.Err()
			case res := <-ch:
				if res.conn != nil {
					// Handshake succeeded; clear the deadline so it doesn't
					// kill the long-lived proxied stream.
					_ = res.conn.SetDeadline(time.Time{})
				}
				return res.conn, res.err
			}
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}, nil
}

// newReverseProxy builds a per-request proxy around a shared transport. The
// transport (and its idle-connection pool) is created once in NewServer;
// only the target URL varies per request.
func newReverseProxy(fixedIP string, targetPort int, transport http.RoundTripper) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(fixedIP, strconv.Itoa(targetPort))}
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(target)
			req.Out.Host = req.In.Host
			req.SetXForwarded()
		},
	}
}
