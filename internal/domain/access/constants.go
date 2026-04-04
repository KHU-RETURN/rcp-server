package access

// SSH channel types
const (
	channelTypeSession = "session"
	channelTypeDirect  = "direct-tcpip"
)

// SSH session request types
const (
	requestPTY          = "pty-req"
	requestShell        = "shell"
	requestExec         = "exec"
	requestWindowChange = "window-change"
	requestExitStatus   = "exit-status"
)

// permissionEmailKey is the gossh.Permissions.Extensions key used to carry
// the authenticated user's email from the auth callback to the connection handler.
const permissionEmailKey = "email"

// vmDefaultUser is the SSH username RCP uses when connecting to OpenStack VMs.
// OpenStack Ubuntu images default to the "ubuntu" system user.
const vmDefaultUser = "ubuntu"
