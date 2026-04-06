# RCP-Server (Return Cloud Platform)

학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.  
현재 README는 프로젝트의 구조와 개발 규칙을 중심으로 설명하며, 도메인별 상세 구현 내용은 추후 별도로 정리할 예정입니다.

## Project Structure & Convention

프로젝트는 진입점, 서버 조립, 도메인 로직, 외부 인프라 연동을 분리하는 구조로 작성되어 있습니다.

### Directory Layout

```text
.
|-- cmd/
|   `-- api/
|       `-- main.go
|-- internal/
|   |-- domain/
|   |   `-- compute/
|   |       |-- handler.go
|   |       |-- init.go
|   |       |-- repository.go
|   |       |-- service.go
|   |       `-- types.go
|   |-- infrastructure/
|   |   |-- http/
|   |   |   `-- client.go
|   |   `-- openstack/
|   |       `-- client.go
|   `-- server/
|       |-- app.go
|       `-- router.go
|-- go.mod
`-- go.sum
```

### Directory Roles

| 경로 | 역할 |
|------|------|
| `cmd/api/` | 애플리케이션 진입점입니다. 환경 변수를 읽고 서버를 실행합니다. |
| `internal/server/` | 애플리케이션 조립과 공통 라우팅을 담당합니다. |
| `internal/domain/compute/` | 도메인 단위 구현이 위치하는 예시 디렉터리입니다. |
| `internal/infrastructure/openstack/` | OpenStack 인증용 ProviderClient를 생성합니다. |
| `internal/infrastructure/http/` | Cloudflare Access 헤더를 주입하는 HTTP 클라이언트를 제공합니다. |

### Domain File Convention

`internal/domain/{domain}` 아래 파일은 다음 역할로 나뉩니다.

| 파일 | 역할 |
|------|------|
| `handler.go` | HTTP 요청/응답 처리 |
| `service.go` | 도메인 비즈니스 로직 |
| `repository.go` | 외부 데이터 소스 접근 |
| `types.go` | 응답/전달용 타입 정의 |
| `init.go` | 저장소, 서비스, 핸들러 조립 |

## Application Flow

현재 애플리케이션의 기본 흐름은 다음과 같습니다.

1. `cmd/api/main.go`에서 `.env`를 로드하고 애플리케이션 실행 준비를 합니다.
2. `internal/infrastructure/openstack/`에서 외부 인프라 클라이언트를 생성합니다.
3. `internal/server/`에서 앱 의존성을 조립하고 라우터를 초기화합니다.
4. 각 도메인은 `handler -> service -> repository` 흐름으로 요청을 처리합니다.

## Development Rules

- 새로운 기능은 원칙적으로 `internal/domain/{service}` 하위에 추가합니다.
- HTTP 처리, 비즈니스 로직, 외부 연동 코드를 한 파일에 섞지 않습니다.
- 도메인 간 결합이 필요할 경우 직접 구현을 참조하기보다 인터페이스나 조립 계층을 통해 연결하는 방향을 우선합니다.
- README에는 계획 중인 스택보다 현재 저장소에 실제로 존재하는 구현을 기준으로 기록합니다.

## Tech Stack

현재 코드와 `go.mod` 기준 핵심 스택은 다음과 같습니다.

| 항목 | 사용 기술 |
|------|-----------|
| Language | Go 1.26.1 |
| Web Framework | `gin-gonic/gin` |
| OpenStack SDK | `gophercloud/gophercloud` |
| Env Loader | `joho/godotenv` |

## Environment Variables

| 변수 | 설명 |
|------|------|
| `PORT` | 서버 포트. 비어 있으면 `8080`을 사용합니다. |
| `OS_AUTH_URL` | OpenStack Identity 엔드포인트 |
| `OS_USERNAME` | OpenStack 사용자 이름 |
| `OS_PASSWORD` | OpenStack 비밀번호 |
| `OS_PROJECT_NAME` | OpenStack 프로젝트 이름 |
| `OS_USER_DOMAIN_NAME` | OpenStack 사용자 도메인 이름 |
| `CF_ACCESS_CLIENT_ID` | Cloudflare Access Client ID |
| `CF_ACCESS_CLIENT_SECRET` | Cloudflare Access Client Secret |

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

## Domain Documentation

도메인별 API, 요청/응답 예시, 세부 비즈니스 규칙은 구현이 더 정리된 뒤 별도 섹션 또는 문서로 추가할 예정입니다.

## Notes

- OpenStack 호출은 Cloudflare Access 헤더가 포함된 HTTP 클라이언트를 통해 수행됩니다.
- 아직 테스트 코드는 없고, 현재 검증은 `go test ./...` 수준의 컴파일 검증에 가깝습니다.
