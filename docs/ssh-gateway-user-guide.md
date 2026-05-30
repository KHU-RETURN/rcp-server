# SSH Gateway User Guide

## Prerequisites

- OpenSSH client
- `cloudflared`
- An SSH key loaded into `ssh-agent`

## SSH Config

Add this to `~/.ssh/config`:

```sshconfig
Host rcp-gw rcp-gw.return.dev
  HostName rcp-gw.return.dev
  User any
  ForwardAgent yes
  ProxyCommand cloudflared access ssh --hostname %h
```

## Connect

```bash
ssh rcp-gw
```

The terminal prints an auth URL and a six-digit code:

```text
Open: https://rcp.return.dev/ssh-auth?s=<nonce>
Code: 123456
```

Open the URL, enter the code shown in the same terminal, and finish Google OAuth. After auth, choose a VM from the terminal menu. If you have one VM, the gateway selects it automatically.

## Notes

- Use `ssh -A` or `ForwardAgent yes`; the gateway does not store private keys.
- Keep the six-digit code private. The URL alone is not enough to authorize a session.
- If host key verification fails, ask an operator to register the VM host key for the gateway.
