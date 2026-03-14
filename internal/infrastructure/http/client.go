package http

import "net/http"

type cloudflareTransport struct {
	rt           http.RoundTripper
	clientID     string
	clientSecret string
}

func (t *cloudflareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("CF-Access-Client-Id", t.clientID)
	req.Header.Set("CF-Access-Client-Secret", t.clientSecret)
	return t.rt.RoundTrip(req)
}

func NewCloudflareClient(id, secret string) *http.Client {
	return &http.Client{
		Transport: &cloudflareTransport{
			rt:           http.DefaultTransport,
			clientID:     id,
			clientSecret: secret,
		},
	}
}