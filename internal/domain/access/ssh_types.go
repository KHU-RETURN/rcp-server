package access

// Wire contract between RCP API and ssh-gateway over the local notify socket.
// Both sides import these so a rename can't drift one half of the protocol.
const (
	NotifySigHeader = "X-RCP-Notify-Sig"
	NotifyPath      = "/notify"
)

type NotifyRequest struct {
	Nonce     string `json:"nonce"`
	Code      string `json:"code"`
	UserEmail string `json:"user_email"`
}
