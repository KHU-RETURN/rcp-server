RCP-Server (Return Cloud Platform)
학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.

📁 Project Structure & Convention
프로젝트의 일관성을 위해 다음과 같은 디렉토리 구조 및 명명 규칙을 따릅니다.

📍 Directory Layout
cmd/api/: 애플리케이션 진입점 및 Gin 라우터 초기화

internal/: 외부에서 접근 불가능한 비즈니스 로직

domain/: 도메인 중심 설계(DDD)에 기반한 서비스 단위 분리

{domain_name}/: 각 서비스 도메인 (예: compute, network, storage)

handler.go: API 엔드포인트 및 요청 핸들링 (Gin)

service.go: 핵심 비즈니스 로직 및 인터페이스 정의

repository.go: 데이터 소스 접근 로직 (Ent)

pkg/: 프로젝트 전반에서 재사용 가능한 유틸리티 라이브러리

configs/: 환경 설정 파일 (.yaml, .env)

api/: API 명세서 (Swagger/OpenAPI)

📏 Development Rules
새로운 기능 추가 시 internal/domain/{service} 하위에 구현하는 것을 원칙으로 합니다.

도메인 간의 의존성은 인터페이스를 통해 결합도를 낮춥니다.

🛠 Tech Stack & Versions
🏁 Language & Runtime
Go: 1.26.1 (darwin/arm64)

🌍 Infrastructure (Core)
OpenStack SDK: gophercloud/gophercloud v1.14.1

Web Framework: gin-gonic/gin v1.12.0

Database (ORM): entgo.io/ent v0.14.5 (Schema-first)

Task Queue: hibiken/asynq v0.26.0 (Redis-based)

🔐 Security & Communication
Auth: golang-jwt/jwt/v5 v5.3.1

Real-time: gorilla/websocket v1.5.3

Policy: casbin/casbin (RBAC - To be implemented)

📊 Observability & Tools
Logging: uber-go/zap (Structured Logging)

Config: spf13/viper v1.21.0

Lint: golangci-lint
