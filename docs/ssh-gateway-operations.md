# SSH Gateway Operations

## Binaries

- `cmd/api`: handles Google OAuth and notifies the gateway over a local Unix socket.
- `cmd/ssh-gateway`: accepts user SSH sessions, waits for OAuth, lists owned VMs, and relays to the selected VM through `cmd/ns-proxy`.
- `cmd/ns-proxy`: SOCKS5 transport inside the OpenStack tenant namespace.

## Runtime Users And Sockets

Run `rcp-server`, `ssh-gateway`, and `ns-proxy` with the shared `return` group. Both Unix sockets are `0660`, so group membership is required:

- `/run/rcp/ssh-gateway-notify.sock`: API -> ssh-gateway
- `/run/rcp/ns-proxy.sock`: ssh-gateway -> ns-proxy

## Required API Env

- `FRONTEND_URL` or `RCP_FRONTEND_BASE_URL`
- `RCP_VM_KNOWN_HOSTS_PATH` or `RCP_SSH_GW_KNOWN_HOSTS_PATH`
- `RCP_SSH_GW_NOTIFY_SOCK`
- `RCP_SSH_GW_NOTIFY_SECRET`

If the notify envs are absent, normal web auth still works but SSH OAuth callback notification is disabled.

## Required Gateway Env

- `RCP_SSH_GW_NOTIFY_SECRET`
- `RCP_SSH_GW_AUTH_URL_BASE`
- `RCP_SSH_GW_API_URL_BASE` when the API is not reachable at the same origin as `RCP_SSH_GW_AUTH_URL_BASE`
- `RCP_SSH_GW_DB_PATH` for the API-owned SQLite database. Use `DB_DSN` only for a manual gateway-specific database override.
- OpenStack auth envs used by the existing provider client
- `RCP_SSH_GW_FIXED_NETWORK` when VMs can have fixed IPs on more than one OpenStack network
- `RCP_SSH_GW_VM_USERS` to change the default SSH login fallback order (`ubuntu,rocky`)

The gateway opens Ent without running schema migration. Schema ownership stays with the API process, so the API must start and complete migration before the gateway is exposed.

## Host Keys

The gateway verifies inner VM SSH host keys with `RCP_SSH_GW_KNOWN_HOSTS_PATH`, defaulting to `/etc/rcp/ssh-gateway/known_hosts`. Missing or mismatched keys fail closed.

Register fixed-IP host keys before users connect:

```bash
ssh-keyscan 10.0.0.7 | sudo tee -a /etc/rcp/ssh-gateway/known_hosts
sudo chown return:return /etc/rcp/ssh-gateway/known_hosts
sudo chmod 0640 /etc/rcp/ssh-gateway/known_hosts
```

## Deploy Checklist

1. Install/update `server`, `ssh-gateway`, and `ns-proxy` binaries.
2. Install systemd units from `deploy/systemd/`.
3. Populate `/etc/rcp/ssh-gateway.env`.
4. Populate `/etc/cloudflared/config.yml`.
5. Register VM host keys in the gateway known_hosts file.
6. Restart in order: `ns-proxy`, `rcp-server`, `ssh-gateway`, `cloudflared`.

Keep `ssh-gateway` and `cloudflared` behind the API restart on first deploys or schema-affecting deploys, because `rcp-server` owns Ent migrations and `ssh-gateway` opens the database without migration.

`deploy-ssh-gateway.yml` builds and installs the gateway binary and systemd unit. Cloudflared tunnel credentials and DNS routing remain a one-time host setup using `deploy/cloudflared/config.yml.example`.
