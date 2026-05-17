# RCP-Server (Return Cloud Platform)

학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.

프로젝트 구조, 개발 규칙, 코드 컨벤션, API 엔드포인트 등 상세한 내용은 [GUIDELINE.md](GUIDELINE.md)를 참고하세요.

## 아키텍처
<img width="1739" height="904" alt="아키텍처" src="https://github.com/user-attachments/assets/782ec4df-ca55-4f6f-a096-29f2f5cb7419" />

## Run

환경 변수를 준비한 뒤 아래 명령으로 실행합니다.

```bash
go run ./cmd/api
```

기본 주소는 `http://localhost:8080`입니다.

## API Docs Generation

Swagger 문서는 Swaggo 주석 기반으로 생성합니다.

문서를 갱신하려면:

```bash
go generate ./cmd/api
```

생성된 산출물은 `docs/generated/swagger.yaml`이며, 서버 실행 후 다음 경로에서 확인할 수 있습니다.

- `http://localhost:8080/docs`
- `http://localhost:8080/openapi.yaml`

## Deployment

`main` 브랜치 머지 시 `compute-1` 호스트로 자동 배포됩니다.

- `.github/workflows/deploy.yml` — rcp-server (매 머지마다)
- `.github/workflows/deploy-ns-proxy.yml` — ns-proxy (`cmd/ns-proxy/**`나 `deploy/systemd/ns-proxy.service` 변경 시, 또는 `workflow_dispatch` 수동 trigger)

### 기여할 때 알아둘 것

- **새 환경변수 추가** — `cmd/api/main.go`의 `os.Getenv` + `deploy.yml`의 `envs:`/`printf` 블록 + GitHub Secrets, 세 군데를 같이 갱신해야 합니다
- **systemd unit 변경** — `deploy/systemd/*.service`는 IaC로 관리됩니다. 머지하면 호스트의 `/etc/systemd/system/`에 자동 install + `daemon-reload` + `restart`가 일어나 즉시 운영에 반영됩니다
- **ent schema 변경** — `NewEntClient`가 시작 시 `Schema.Create`를 자동 호출하므로 서버 재시작이 곧 마이그레이션입니다. prod DB와 호환되는 변경인지 확인 필요
- **ns-proxy 의존 코드 변경** — `internal/` 등 공유 코드를 바꿨다면 ns-proxy 워크플로는 자동 trigger되지 않으니 GitHub Actions에서 `Run workflow`로 수동 실행
- **OpenStack 라우터(qrouter) UUID 변경** — `RCP_NS_PROXY_ROUTER_ID` Secret 갱신 후 ns-proxy 워크플로 수동 재실행
- **운영 로그 조회** — `ssh return@compute-1 journalctl -u rcp-server -f` (ns-proxy도 동일 패턴)
- **로컬 개발** — 프로젝트 루트의 `.env`를 godotenv가 로드합니다. 운영에서는 systemd `EnvironmentFile`이 같은 역할을 하므로 동작이 일치합니다

## Notes

- OpenStack 호출은 Cloudflare Access 헤더가 포함된 HTTP 클라이언트를 통해 수행됩니다.
- 아직 테스트 코드는 없고, 현재 검증은 `go test ./...` 수준의 컴파일 검증에 가깝습니다.
