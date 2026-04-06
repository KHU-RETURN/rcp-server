# PR Title

feat: automate local Scalar API docs from code annotations

# PR Body

## 관련 내용
- 로컬 개발 중 API 요청/응답 스키마를 브라우저에서 쉽게 확인할 수 있도록 Scalar 기반 API Reference를 유지하면서, 문서 원본을 수동 OpenAPI 파일에서 코드 기반 자동 생성으로 전환했습니다.

## 변경 사항
- 로컬 개발용 문서 라우트 유지
  - `GET /docs`
  - `GET /openapi.yaml`
- `/docs`는 기존처럼 Scalar UI를 사용하고, `/openapi.yaml`은 이제 생성된 Swagger YAML을 서빙하도록 변경
- 수동 `openapi.yaml` 제거
- Swaggo 기반 문서 자동 생성 도입
  - `go generate ./cmd/api`
  - 생성 산출물: `docs/generated/swagger.yaml`
- 현재 공개 API에 최소 Swaggo annotation 추가
  - `POST /api/v1/access/keypairs`
  - `GET /api/v1/compute/flavors`
  - `GET /api/v1/compute/flavors/all`
  - `GET /api/v1/compute/flavors/available`
  - `POST /api/v1/compute/instances`
- 기존 요청/응답 계약을 이름 있는 스키마로 정리
  - `CreateInstanceRequest`
  - `CreateInstanceResponse`
  - 공통 `ErrorResponse`
- `POST /api/v1/compute/instances`는 더 이상 SDK `servers.Server`를 핸들러에서 직접 노출하지 않고, 현재 JSON shape를 그대로 유지하는 로컬 응답 스키마로 변환
- GitHub Actions에 문서 최신성 검증 추가
  - `go generate ./cmd/api`
  - `git diff --exit-code`

## 기대 효과
- 코드 변경 시 문서와 실제 API 계약이 덜 어긋나게 됩니다.
- 개발자는 `/docs`에서 메서드, 경로, 요청 바디, 응답 스키마를 바로 확인할 수 있습니다.
- 문서 생성 절차가 명확해지고, CI에서 누락을 자동으로 잡을 수 있습니다.

## 사용 방법
```bash
go generate ./cmd/api
go run ./cmd/api
```

- Docs UI: `http://localhost:8080/docs`
- Raw spec: `http://localhost:8080/openapi.yaml`

## 테스트
- `go generate ./cmd/api`
- `go test ./...`
- `go vet ./...`
- `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.3 run`
- `go mod tidy -diff`

## 참고 사항
- `/docs`, `/openapi.yaml`는 release 모드에서는 노출되지 않습니다.
- Scalar UI는 CDN을 사용하므로 인터넷 연결이 없으면 문서 화면이 렌더링되지 않을 수 있습니다.
- 문서 자동 생성은 Swaggo annotation 기반이므로, 공개 API 계약 변경 시 annotation과 `docs/generated/swagger.yaml`을 함께 갱신해야 합니다.
