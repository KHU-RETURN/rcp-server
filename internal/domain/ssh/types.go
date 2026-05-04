package ssh

// NotifyRequest is the payload posted to the gateway's /notify endpoint.
type NotifyRequest struct {
	Nonce     string `json:"nonce"`
	UserEmail string `json:"user_email"`
}
