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

## Generated Artifacts

Swagger 문서는 Swaggo 주석 기반으로 생성합니다.

API 문서를 갱신하려면:

```bash
go generate ./cmd/api
```

생성된 산출물은 `docs/generated/swagger.yaml`이며, 서버 실행 후 다음 경로에서 확인할 수 있습니다.

- `http://localhost:8080/docs`
- `http://localhost:8080/openapi.yaml`

ent schema 변경 후 ent client/migration 산출물을 갱신하려면:

```bash
go run entc.go
```

PR CI는 Swagger와 ent 산출물을 재생성한 뒤 `git diff --exit-code`로 커밋 누락을 검사합니다.

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

터미널에 표시된 6자리 코드를 브라우저 인증 화면에 입력한 뒤 OAuth 로그인하면 본인 VM 리스트가 표시됩니다. 선택하면 `cmd/ns-proxy`를 통해 tenant network의 VM에 셸 세션이 연결됩니다. RCP는 사용자 private key를 보관하지 않고 forwarding된 ssh-agent로만 인증합니다.

운영 가이드: [docs/ssh-gateway-operations.md](docs/ssh-gateway-operations.md)
사용자 가이드: [docs/ssh-gateway-user-guide.md](docs/ssh-gateway-user-guide.md)

## Deployment

`main` 브랜치 머지 시 `compute-1` 호스트로 자동 배포됩니다.

- `.github/workflows/deploy.yml` — rcp-server (매 머지마다)
- `.github/workflows/deploy-ns-proxy.yml` — ns-proxy (`cmd/ns-proxy/**`, `internal/utils/**`, `deploy/systemd/ns-proxy.service`, workflow 변경 시, 또는 `workflow_dispatch` 수동 trigger)
- `.github/workflows/deploy-ssh-gateway.yml` — ssh-gateway (`cmd/ssh-gateway/**`, SSH gateway 관련 access/database/openstack 코드, `internal/utils/**`, systemd example, workflow 변경 시, 또는 `workflow_dispatch` 수동 trigger)

### 기여할 때 알아둘 것

- **새 환경변수 추가** — `cmd/api/main.go`의 `os.Getenv` + `deploy.yml`의 `envs:`/`printf` 블록 + GitHub Secrets, 세 군데를 같이 갱신해야 합니다
- **systemd unit 변경** — `deploy/systemd/*.service`는 IaC로 관리됩니다. 머지하면 호스트의 `/etc/systemd/system/`에 자동 install + `daemon-reload` + `restart`가 일어나 즉시 운영에 반영됩니다
- **ent schema 변경** — `go run entc.go`로 generated code를 갱신해야 합니다. 런타임에서는 `NewEntClient`가 시작 시 `Schema.Create`를 자동 호출하므로 서버 재시작이 곧 마이그레이션입니다. prod DB와 호환되는 변경인지 확인 필요
- **ns-proxy 의존 코드 변경** — 자동 trigger path 밖의 공유 코드를 바꿨다면 ns-proxy 워크플로는 자동 trigger되지 않으니 GitHub Actions에서 `Run workflow`로 수동 실행
- **OpenStack 라우터(qrouter) UUID 변경** — `RCP_NS_PROXY_ROUTER_ID` Secret 갱신 후 ns-proxy 워크플로 수동 재실행
- **운영 로그 조회** — `ssh return@compute-1 journalctl -u rcp-server -f` (ns-proxy도 동일 패턴)
- **로컬 개발** — 프로젝트 루트의 `.env`를 godotenv가 로드합니다. 운영에서는 systemd `EnvironmentFile`이 같은 역할을 하므로 동작이 일치합니다

## Notes

- OpenStack 호출은 Cloudflare Access 헤더가 포함된 HTTP 클라이언트를 통해 수행됩니다.
- 단위 테스트는 `go test ./...`로 실행합니다 — `cmd/ns-proxy`, `cmd/ssh-gateway`, `internal/domain/{auth,access,compute,storage}`, `internal/server` 등에 커버리지가 있습니다.
