package http

import (
	"net/http"
	"time"
)

const defaultTimeout = 10 * time.Second

type cloudflareTransport struct {
	rt           http.RoundTripper
	clientID     string
	clientSecret string
}

func (t *cloudflareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clonedReq := req.Clone(req.Context())
	clonedReq.Header.Set("CF-Access-Client-Id", t.clientID)
	clonedReq.Header.Set("CF-Access-Client-Secret", t.clientSecret)
	return t.rt.RoundTrip(clonedReq)
}

func NewCloudflareClient(id, secret string) *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		Transport: &cloudflareTransport{
			rt:           http.DefaultTransport,
			clientID:     id,
			clientSecret: secret,
		},
	}
}
