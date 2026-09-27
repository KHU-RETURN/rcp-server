#!/usr/bin/env bash
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo 'run as root' >&2
  exit 1
fi
if ! command -v docker >/dev/null || ! getent group docker >/dev/null || ! getent group return >/dev/null; then
  echo 'Docker and the return group are required' >&2
  exit 1
fi
script_dir="$(cd "$(dirname "$0")" && pwd)"
binary_path="${1:-$script_dir/function-builder}"
if [[ ! -f "$binary_path" || ! -f "$script_dir/python-3.11.4.wasm" ]]; then
  echo 'run setup.sh and provide the function-builder binary first' >&2
  exit 1
fi
if ! id return-builder >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin --gid return --groups docker return-builder
fi
install -d -m 0755 /opt/rcp-function-builder
install -m 0755 "$binary_path" /opt/rcp-function-builder/function-builder
install -m 0644 "$script_dir/python-3.11.4.wasm" /opt/rcp-function-builder/python-3.11.4.wasm
install -m 0644 "$script_dir/../systemd/rcp-function-builder.service" /etc/systemd/system/rcp-function-builder.service
systemctl daemon-reload
systemctl enable --now rcp-function-builder.service
