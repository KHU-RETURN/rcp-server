# RCP-Server (Return Cloud Platform)

학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.

프로젝트 구조, 개발 규칙, 코드 컨벤션, API 엔드포인트 등 상세한 내용은 [GUIDELINE.md](GUIDELINE.md)를 참고하세요.

## Run

세 개의 바이너리로 구성됩니다.

| 바이너리 | 역할 |
|----------|------|
| `cmd/api` | REST API 서버 (Gin, OpenStack 연동, OAuth) |
| `cmd/ns-proxy` | tenant 네트워크로의 SOCKS5 게이트웨이 (qrouter netns, Unix socket) |
| `cmd/ssh-gateway` | 사용자 → VM SSH 베스천 (OAuth keyboard-interactive + ns-proxy 릴레이) |

환경 변수를 준비한 뒤 아래 명령으로 실행합니다.

```bash
go run ./cmd/api          # 기본 :8080
go run ./cmd/ns-proxy     # /run/rcp/ns-proxy.sock
go run ./cmd/ssh-gateway  # 기본 127.0.0.1:2222 (cloudflared 뒤)
```

API 기본 주소는 `http://localhost:8080`, SSH 게이트웨이는 `127.0.0.1:2222`(외부 직접 노출 X, cloudflared 터널 경유)입니다.

## API Docs Generation

Swagger 문서는 Swaggo 주석 기반으로 생성합니다.

문서를 갱신하려면:

```bash
go generate ./cmd/api
```

생성된 산출물은 `docs/generated/swagger.yaml`이며, 서버 실행 후 다음 경로에서 확인할 수 있습니다.

- `http://localhost:8080/docs`
- `http://localhost:8080/openapi.yaml`

## SSH Access

사용자는 표준 OpenSSH 클라이언트 + `cloudflared`로 본인 VM에 접속합니다. 게이트웨이는 호스트 로컬(`127.0.0.1:2222`)에서만 listen하고, 외부 트래픽은 Cloudflare Tunnel이 `rcp-gw.return.dev`를 그 소켓으로 라우팅합니다.

```sshconfig
# ~/.ssh/config
Host rcp-gw rcp-gw.return.dev
  HostName rcp-gw.return.dev
  User any
  ForwardAgent yes
  ProxyCommand cloudflared access ssh --hostname %h
```

```bash
ssh rcp-gw
```

브라우저에서 OAuth 로그인 후 본인 VM 리스트가 표시되며, 선택하면 `cmd/ns-proxy`를 통해 tenant network의 VM에 셸 세션이 연결됩니다. RCP는 사용자 private key를 보관하지 않고 forwarding된 ssh-agent로만 인증합니다.

운영 가이드: [docs/ssh-gateway-operations.md](docs/ssh-gateway-operations.md)
사용자 가이드: [docs/ssh-gateway-user-guide.md](docs/ssh-gateway-user-guide.md)

## Notes

- OpenStack 호출은 Cloudflare Access 헤더가 포함된 HTTP 클라이언트를 통해 수행됩니다.
- 단위 테스트는 `go test ./...`로 실행합니다 — `cmd/ns-proxy`, `cmd/ssh-gateway`, `internal/domain/{auth,access,compute,ssh}`, `internal/server` 등에 커버리지가 있습니다.
