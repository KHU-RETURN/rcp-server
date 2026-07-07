package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

type unixForwarder struct {
	path string
}

func (u *unixForwarder) Dial(_, _ string) (net.Conn, error) {
	return net.Dial("unix", u.path)
}

func transportViaNSProxy(sockPath string) (*http.Transport, error) {
	forward := &unixForwarder{path: sockPath}
	dialer, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		return nil, err
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
				conn, err := dialer.Dial(network, address)
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
