# Development Guideline

RCP-Server 프로젝트의 구조, 개발 규칙, 코드 컨벤션, API 엔드포인트를 정리한 문서입니다.

---

## 1. 프로젝트 구조

진입점 / 서버 조립 / 도메인 로직 / 외부 인프라 연동을 분리합니다.

```text
.
|-- cmd/api/                  # 진입점: 환경 변수 로드, 서버 실행
|-- internal/
|   |-- api/                  # API 버전 상수(BasePath), 공통 응답 타입(ErrorResponse)
|   |-- domain/               # 도메인별 구현
|   |   |-- access/
|   |   |-- auth/
|   |   |-- compute/
|   |   `-- storage/
|   |-- infrastructure/       # 외부 시스템 연동
|   |   |-- database/         #   SQLite 연결
|   |   |-- http/             #   Cloudflare Access 헤더 주입 HTTP 클라이언트
|   |   `-- openstack/        #   OpenStack ProviderClient 생성
|   `-- server/               # App 조립(app.go), 라우터 초기화(router.go)
|-- go.mod
`-- go.sum
```

### 도메인 파일 구성

각 `internal/domain/{domain}/` 안에서 역할별로 파일을 나눕니다.

| 파일 | 역할 |
|------|------|
| `handler.go` | HTTP 요청/응답 처리 |
| `service.go` | 비즈니스 로직 |
| `repository.go` | 외부 데이터 소스 접근 |
| `types.go` | 요청/응답/전달용 타입 정의 |
| `init.go` | Client → Service → Handler 조립 |

---

## 2. 요청 처리 흐름

```
main.go → infrastructure 클라이언트 생성 → App 조립 → 라우터 초기화
                                                         ↓
                                    handler → service → client/repository
```

1. `cmd/api/main.go`에서 `.env` 로드 후 인프라 클라이언트(OpenStack, DB, OAuth)를 생성합니다.
2. `internal/server/app.go`에서 각 도메인의 `Init()`을 호출해 의존성을 조립합니다.
3. `internal/server/router.go`에서 라우트 그룹과 미들웨어를 설정합니다.
4. 요청이 들어오면 `handler → service → client/repository` 순서로 처리됩니다.

---

## 3. 개발 규칙

- 새 기능은 `internal/domain/{도메인}/` 하위에 추가합니다.
- HTTP 처리 / 비즈니스 로직 / 외부 연동 코드를 한 파일에 섞지 않습니다.
- 도메인 간 직접 참조는 순환 의존이 생기지 않는 한 허용합니다. 순환이 발생하면 인터페이스로 분리합니다.
- 문서에는 계획이 아닌 현재 존재하는 구현만 기록합니다.

---

## 4. 코드 컨벤션

### 의존성 주입

- 인터페이스가 필요한 경우(테스트 mock, 구현이 2개 이상) **사용하는 쪽에서 패키지-스코프(소문자)로 정의**합니다.
- 각 도메인의 `init.go`에서 `Client → Service → Handler` 순서로 와이어링합니다.
- Handler는 Service를, Service는 Client(또는 Repository)를 생성자 주입으로 받습니다.

### 에러 처리

3단계로 나누어 처리합니다.

| 레이어 | 하는 일 |
|--------|---------|
| 인프라 (client) | 외부 SDK 에러를 `StatusError` 같은 커스텀 타입으로 변환 |
| 서비스 | Sentinel 에러(`var ErrXxx = errors.New(...)`) 정의, `fmt.Errorf("%w")`로 래핑 |
| 핸들러 | `switch`로 도메인 에러 → HTTP 상태 코드 매핑 |

공통 에러 응답은 `internal/api/types.go`의 `ErrorResponse`를 사용합니다.

### 요청 / 응답

- **바인딩**: `c.ShouldBindJSON()` + `binding:"required"` 태그
- **정제**: 서비스 레이어에서 `strings.TrimSpace()` 등으로 입력을 다듬은 뒤 검증
- **DTO 분리**: 도메인 모델(`Server`)과 HTTP 응답 타입(`InstanceDetailResponse`)을 구분
- **상태 코드**: 생성 → `201 Created`, 삭제 → `204 No Content`

### 테스트

- Fake 구조체에 함수 포인터 필드를 두어 인터페이스를 구현합니다.
- `t.Run("설명", ...)` 서브테스트 패턴을 사용합니다.
- `httptest.NewRequest()` + `httptest.NewRecorder()` + 테스트별 `gin.New()`로 HTTP를 테스트합니다.
- 외부 어서션 라이브러리 없이 `if` + `errors.Is()`로 검증합니다.

### 네이밍

| 대상 | 규칙 | 예시 |
|------|------|------|
| 패키지 | 소문자, 도메인 기반 | `auth`, `compute`, `access` |
| 공개 타입 | 첫 글자 대문자 | `Handler`, `Service`, `KeyPair` |
| DTO | `Response` / `Request` 접미사 | `CreateKeyPairRequest` |
| 도메인 모델 | 접미사 없음 | `Server`, `Flavor` |
| 핸들러 메서드 | HTTP 동사 프리픽스 | `GetInstanceDetail`, `ListKeyPairs` |
| 테스트 Fake | 소문자 | `fakeClient`, `fakeRepo` |

---

## 5. API 엔드포인트

모든 엔드포인트는 `/api/v1` (`api.BasePath`) 아래에 구성됩니다.

### Auth (인증 불필요)

| Method | Path | Handler | 설명 |
|--------|------|---------|------|
| GET | `/api/v1/auth/oauth/google` | Login | 구글 로그인 페이지로 리다이렉트 |
| GET | `/api/v1/auth/oauth/google/callback` | Callback | 구글 로그인 콜백 |

### Compute (인증 필요)

| Method | Path | Handler | 설명 |
|--------|------|---------|------|
| GET | `/api/v1/compute/flavors` | GetFlavors | Flavor 목록 조회 |
| GET | `/api/v1/compute/instances` | GetInstances | 인스턴스 목록 조회 |
| GET | `/api/v1/compute/instances/:id` | GetInstanceDetail | 인스턴스 상세 조회 |
| POST | `/api/v1/compute/instances` | CreateInstance | 인스턴스 생성 |
| DELETE | `/api/v1/compute/instances/:id` | DeleteInstance | 인스턴스 삭제 |

### Access (인증 필요)

| Method | Path | Handler | 설명 |
|--------|------|---------|------|
| GET | `/api/v1/access/keypairs` | ListKeyPairs | 키페어 목록 조회 |
| POST | `/api/v1/access/keypairs` | CreateKeyPair | 키페어 생성 |
| GET | `/api/v1/access/keypairs/:name` | GetKeyPair | 키페어 조회 |
| DELETE | `/api/v1/access/keypairs/:name` | DeleteKeyPair | 키페어 삭제 |

### Storage (인증 필요)

| Method | Path | Handler | 설명 |
|--------|------|---------|------|
| GET | `/api/v1/storage/containers` | ListContainers | 내 container 목록 조회 |
| POST | `/api/v1/storage/containers` | CreateContainer | container 생성 |
| DELETE | `/api/v1/storage/containers/:name` | DeleteContainer | container 삭제 (`?force=true`로 강제 삭제) |
| GET | `/api/v1/storage/containers/:name/objects` | ListObjects | object 목록 조회 |
| POST | `/api/v1/storage/containers/:name/objects/*key` | UploadObject | 파일 업로드 (multipart/form-data, key=`file`) |
| GET | `/api/v1/storage/containers/:name/objects/*key` | DownloadObject | 파일 다운로드 (스트리밍) |
| GET | `/api/v1/storage/containers/:name/archive?prefix=path/` | ArchiveObjects | prefix 아래 object를 zip으로 다운로드 |
| DELETE | `/api/v1/storage/containers/:name/objects/*key` | DeleteObject | 파일 삭제 |

---

## 6. 기술 스택

| 항목 | 사용 기술 |
|------|-----------|
| Language | Go 1.26.1 |
| Web Framework | `gin-gonic/gin` |
| OpenStack SDK | `gophercloud/gophercloud` |
| Env Loader | `joho/godotenv` |

---

## 7. 환경 변수

바이너리별로 분리해 표기합니다. **필수** 표시가 없는 항목은 기본값이 있어 생략 가능합니다.

### 7.1 `cmd/api`

| 변수 | 의무 | 설명 |
|------|------|------|
| `PORT` | optional (기본 `8080`) | HTTP 리스닝 포트 |
| `RCP_JWT_SECRET` | optional (dev 폴백 있음) | JWT 서명 시크릿. 운영에서는 반드시 설정 |
| `OS_AUTH_URL` | **필수** | OpenStack Identity 엔드포인트 |
| `OS_USERNAME` | **필수** | OpenStack 사용자 이름 |
| `OS_PASSWORD` | **필수** | OpenStack 비밀번호 |
| `OS_PROJECT_NAME` | **필수** | OpenStack 프로젝트 이름 |
| `OS_PROJECT_ID` | **필수** | OpenStack 프로젝트 ID (API에서 scope 지정용) |
| `OS_USER_DOMAIN_NAME` | **필수** | OpenStack 사용자 도메인 |
| `CF_ACCESS_CLIENT_ID` | **필수** | Cloudflare Access Client ID (OpenStack 프록시용) |
| `CF_ACCESS_CLIENT_SECRET` | **필수** | Cloudflare Access Client Secret |
| `GG_OAUTH_CLIENT` | **필수** | Google OAuth Client ID |
| `GG_OAUTH_SECRET` | **필수** | Google OAuth Client Secret |
| `GG_REDIRECT_URL` | **필수** | Google OAuth Redirect URL |
| `DB_DRIVER` | optional (기본 `sqlite3`) | ent DB 드라이버 |
| `DB_DSN` | optional (기본 `file:/var/lib/rcp/rcp.db?...`) | DB DSN |
| `FRONTEND_URL` / `RCP_FRONTEND_BASE_URL` | **필수(둘 중 하나)** | OAuth 콜백 redirect 프런트엔드 origin (예: `https://rcp.return.dev`). 둘 다 있으면 `FRONTEND_URL` 우선 |
| `RCP_VM_KNOWN_HOSTS_PATH` / `RCP_SSH_GW_KNOWN_HOSTS_PATH` | optional | web console과 ssh-gateway가 inner VM host key를 검증할 known_hosts 경로 |
| `RCP_SSH_GW_NOTIFY_SOCK` | optional | ssh-gateway notify Unix socket 경로. 미설정 시 SSH 분기 비활성 |
| `RCP_SSH_GW_NOTIFY_SECRET` | optional | ssh-gateway와 공유하는 HMAC-SHA256 시크릿 |

### 7.2 `cmd/ns-proxy`

| 변수 | 의무 | 설명 |
|------|------|------|
| `RCP_NS_PROXY_SOCK` | optional (기본 `/run/rcp/ns-proxy.sock`) | SOCKS5 Unix socket 경로 |
| `RCP_NS_PROXY_ALLOW_CIDR` | **필수** | dial 허용 CIDR 리스트 (쉼표 구분). 빈 값은 fail-closed로 부팅 거부 |
| `RCP_NS_PROXY_MAX_CONNS` | optional (기본 `1024`) | 동시 연결 상한 |
| `RCP_NS_PROXY_DIAL_TIMEOUT` | optional (기본 `5s`) | 백엔드 dial 타임아웃 |
| `RCP_NS_PROXY_SHUTDOWN_GRACE` | optional (기본 `30s`) | graceful shutdown 대기 |
| `RCP_NS_PROXY_LOG_LEVEL` | optional (기본 `info`) | |

### 7.3 `cmd/ssh-gateway`

| 변수 | 의무 | 설명 |
|------|------|------|
| `OS_AUTH_URL` | **필수** | OpenStack Identity 엔드포인트 |
| `OS_USERNAME` | **필수** | OpenStack 사용자 이름 |
| `OS_PASSWORD` | **필수** | OpenStack 비밀번호 |
| `OS_PROJECT_NAME` | **필수** | OpenStack 프로젝트 이름 |
| `OS_USER_DOMAIN_NAME` | **필수** | OpenStack 사용자 도메인 |
| `CF_ACCESS_CLIENT_ID` | **필수** | Cloudflare Access Client ID (OpenStack 프록시용) |
| `CF_ACCESS_CLIENT_SECRET` | **필수** | Cloudflare Access Client Secret |
| `RCP_SSH_GW_LISTEN` | optional (기본 `127.0.0.1:2222`) | outer SSH 리스닝. 외부 직접 노출하려면 `:2222`로 override (cloudflared 사용 권장) |
| `RCP_SSH_GW_HOST_KEY_PATH` | optional (기본 `/etc/rcp/ssh-gateway/host_ed25519`) | host ed25519 key 영속화 경로 (없으면 생성) |
| `RCP_SSH_GW_KNOWN_HOSTS_PATH` | optional (기본 `/etc/rcp/ssh-gateway/known_hosts`) | inner VM host key 신뢰 저장소. VM 키가 없으면 fail closed |
| `RCP_SSH_GW_NOTIFY_SOCK` | optional (기본 `/run/rcp/ssh-gateway-notify.sock`) | api ↔ gateway notify 소켓 |
| `RCP_SSH_GW_NOTIFY_SECRET` | **필수** | api와 공유 HMAC 시크릿 |
| `RCP_SSH_GW_AUTH_URL_BASE` | **필수** | 사용자 터미널에 출력할 프런트 origin (예: `https://rcp.return.dev`) |
| `RCP_SSH_GW_DB_PATH` | **필수(DB_DSN 미사용 시)** | ent SQLite 경로. gateway는 migration 없이 read/query 용도로 open |
| `DB_DRIVER` / `DB_DSN` | optional | 수동 설치에서 gateway 전용 DSN을 직접 지정할 때만 사용. 배포 워크플로는 공유 `DB_DSN` 대신 `RCP_SSH_GW_DB_PATH`를 전달 |
| `RCP_SSH_GW_NONCE_TTL` | optional (기본 `5m`) | pending-session 만료 |
| `RCP_SSH_GW_MAX_PENDING_SESSIONS` | optional (기본 `1024`) | pending OAuth 세션 전역 상한 |
| `RCP_SSH_GW_FIXED_NETWORK` | optional | multi-network VM에서 fixed IPv4를 고를 OpenStack network 이름 |
| `RCP_SSH_GW_VM_USER` | optional (기본 `root`) | inner VM SSH 로그인 사용자. Ubuntu cloud image 등 root 로그인이 막힌 이미지에서는 `ubuntu` 등으로 설정 |
| `RCP_NS_PROXY_SOCK` | optional (기본 `/run/rcp/ns-proxy.sock`) | ns-proxy SOCKS5 소켓 |
| `RCP_SSH_GW_LOG_LEVEL` | optional (기본 `info`) | |
