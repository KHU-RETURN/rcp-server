# RCP-Server (Return Cloud Platform)
학술동아리 RETURN의 클라우드 관리 플랫폼 백엔드 서버입니다.

## 🛠 Tech Stack & Versions
프로젝트의 안정성과 최신 표준을 위해 버전을 기록해놓겠습니다.

### 🏁 Language & Runtime
- **Go**: `1.26.1` (darwin/arm64)

### 🌍 Infrastructure (Core)
- **OpenStack SDK**: `gophercloud/gophercloud v1.14.1` (Cloud 제어)
- **Web Framework**: `gin-gonic/gin v1.12.0` (API 서버)
- **Database (ORM)**: `entgo.io/ent v0.14.5` (Schema-first ORM)
- **Task Queue**: `hibiken/asynq v0.26.0` (Redis 기반 비동기 작업)

### 🔐 Security & Communication
- **Auth**: `golang-jwt/jwt/v5 v5.3.1` (토큰 기반 인증)
- **Real-time**: `gorilla/websocket v1.5.3` (SSH 중계 및 실시간 통신)
- **Policy**: `casbin/casbin` (RBAC 권한 관리 - 도입 예정)

### 📊 Observability & Tools
- **Logging**: `uber-go/zap` (고성능 구조화 로깅)
- **Config**: `spf13/viper v1.21.0` (환경 변수 관리)
- **Lint**: `golangci-lint` (코드 품질 관리)