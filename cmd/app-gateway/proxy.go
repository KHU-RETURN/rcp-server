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

func newReverseProxy(fixedIP string, targetPort int, sockPath string) (*httputil.ReverseProxy, error) {
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(fixedIP, strconv.Itoa(targetPort))}
	rp := httputil.NewSingleHostReverseProxy(target)
	transport, err := transportViaNSProxy(sockPath)
	if err != nil {
		return nil, err
	}
	rp.Transport = transport

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalHost := req.Host
		originalDirector(req)
		req.Host = originalHost
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
	}
	return rp, nil
}
