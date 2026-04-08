# RCP-Server (Return Cloud Platform)

학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.

프로젝트 구조, 개발 규칙, 코드 컨벤션, API 엔드포인트 등 상세한 내용은 [GUIDELINE.md](GUIDELINE.md)를 참고하세요.

## Run

환경 변수를 준비한 뒤 아래 명령으로 실행합니다.

```bash
go run ./cmd/api
```

기본 주소는 `http://localhost:8080`입니다.

## Code Generation

### Ent ORM

DB 엔티티 스키마는 `internal/schema/`에 정의되어 있습니다. 스키마를 변경한 뒤 아래 명령으로 Ent 클라이언트 코드를 재생성합니다.

```bash
go generate ./ent/...
```

생성된 코드는 `ent/` 디렉토리에 출력되며, `.gitignore`로 버전 관리에서 제외됩니다.

### API Docs Generation

Swagger 문서는 Swaggo 주석 기반으로 생성합니다.

문서를 갱신하려면:

```bash
go generate ./cmd/api
```

생성된 산출물은 `docs/generated/swagger.yaml`이며, 서버 실행 후 다음 경로에서 확인할 수 있습니다.

- `http://localhost:8080/docs`
- `http://localhost:8080/openapi.yaml`

## Notes

- OpenStack 호출은 Cloudflare Access 헤더가 포함된 HTTP 클라이언트를 통해 수행됩니다.
- 아직 테스트 코드는 없고, 현재 검증은 `go test ./...` 수준의 컴파일 검증에 가깝습니다.
