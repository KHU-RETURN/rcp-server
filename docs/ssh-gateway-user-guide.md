# SSH Gateway User Guide

## Prerequisites

- OpenSSH client
- `cloudflared`

## SSH Config

Add this to `~/.ssh/config`:

```sshconfig
Host rcp-gw rcp-gw.return.dev
  HostName rcp-gw.return.dev
  User any
  ProxyCommand cloudflared access ssh --hostname %h
```

## Connect

```bash
ssh rcp-gw
```

The terminal prints a browser-auth prompt:

```text
RCP SSH browser authentication required.

1. Open this URL in your browser:
   https://rcp.return.dev/ssh-auth?s=<nonce>

2. Enter this 6-digit code on the auth page:
123456

If your terminal supports clipboard integration, the code was copied automatically.
Waiting for browser authentication. Timeout: 5m0s
```

Open the URL, enter the code shown in the same terminal, and finish Google OAuth. In terminals that support it, the URL is clickable and the code is copied to your clipboard automatically. After auth, choose a VM from the terminal menu. If you have one VM, the gateway selects it automatically.

## Notes

- The gateway uses a short-lived SSH key for the selected VM; you do not need `ssh -A` or a local VM private key.
- The temporary key is removed when the SSH session ends. If cleanup cannot run, it expires from the API-side authorized-key store.
- Keep the six-digit code private. The URL alone is not enough to authorize a session.
- If host key verification fails, ask an operator to register the VM host key for the gateway.
