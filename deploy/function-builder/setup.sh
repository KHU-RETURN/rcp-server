#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
docker build -t rcp-wasm-rust:1 -f Dockerfile.rust .
docker build -t rcp-wasm-go:1 -f Dockerfile.golang .
docker build -t rcp-wasm-javy:1 -f Dockerfile.javy .
docker pull python:3.11-slim
curl -fL 'https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/python%2F3.11.4%2B20230714-11be424/python-3.11.4.wasm' -o python-3.11.4.wasm
echo '422a1088378ef31788384afcdf705594d833e10c7d6336c6988e2875a269f68b  python-3.11.4.wasm' | sha256sum -c -
