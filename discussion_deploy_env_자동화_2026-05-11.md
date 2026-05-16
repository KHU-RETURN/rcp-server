# deploy.yml env 추가 편의성 개선

날짜: 2026-05-11
관련 PR 리뷰 코멘트: `.github/workflows/deploy.yml` line 62-63 (Choi-Eunseok)
> 이 부분 추가 쉽게 가능하게

## 문제

`Write .env file` step에서 새 환경변수를 추가할 때마다 두 곳을 동기화해야 함:
1. `envs:` 입력의 콤마 구분 리스트
2. `script:` 내부 `printf '%s\n' "VAR=$VAR"` 반복문

리스트가 늘어날수록 누락/오타 가능성 증가.

## 검토한 옵션

### 옵션 A — `toJSON(secrets)` + `jq` prefix 필터링

```yaml
env:
  SECRETS_JSON: ${{ toJSON(secrets) }}
with:
  envs: SECRETS_JSON
  script: |
    printf '%s' "$SECRETS_JSON" | jq -r '
      to_entries[]
      | select(.key | test("^(RCP_|OS_|CF_|GG_|DB_|PORT$)"))
      | "\(.key)=\(.value)"
    ' > ~/rcp-server/.env
```

- 장점: yml 수정 zero. GitHub Secrets에 새 변수 추가하면 prefix만 맞으면 자동 반영.
- 단점: secret 네이밍 prefix 컨벤션 강제. 서버에 `jq` 필요.

### 옵션 B — `envs` + `script` for 루프 (명시적)

`envs:` 리스트와 `script:`의 for 루프, 두 곳 모두 수동 관리.

- 장점: 컨벤션 강제 없음, 매우 명시적.
- 단점: 여전히 두 곳 수정.

### 옵션 C — `INPUT_ENVS` 활용 (한 곳 수정)

`appleboy/ssh-action` 내부 변수 의존 → 미문서화, 액션 버전업 시 깨질 위험.

## 추가로 발견된 문제

기존 yml에 **`secrets.* → step env` 매핑이 누락**되어 있었음.

`appleboy/ssh-action`의 `envs:`는 GitHub Actions runner 프로세스의 환경변수에서 값을 가져옴. 하지만 GitHub secrets는 `${{ secrets.* }}` 표현식으로만 접근 가능하고, 자동으로 env에 노출되지 않음.

→ 기존 deploy.yml은 사실상 `.env` 파일에 **빈 값**으로 작성되고 있었을 가능성이 큼. 운영 서버의 `~/rcp-server/.env` 확인 필요.

## 최종 결정

**옵션 C의 변형**: step `env:` 블록에 `ENV_LIST` 정의 → `with.envs`와 `script:` 모두 `${{ env.ENV_LIST }}`로 참조.

- 새 env 추가 시 수정 위치:
  1. `env:` 블록에 secret 매핑 한 줄 추가
  2. `ENV_LIST`에 변수명 추가
- `script:`의 for 루프는 자동 처리 → 누락/오타 위험 제거.

```yaml
- name: Write .env file
  uses: appleboy/ssh-action@v0.1.10
  env:
    ENV_LIST: RCP_JWT_SECRET,OS_AUTH_URL,...,PORT
    RCP_JWT_SECRET: ${{ secrets.RCP_JWT_SECRET }}
    OS_AUTH_URL: ${{ secrets.OS_AUTH_URL }}
    # ...
  with:
    host: ${{ secrets.SERVER_HOST }}
    username: ${{ secrets.SERVER_USER }}
    key: ${{ secrets.SERVER_KEY }}
    envs: ${{ env.ENV_LIST }}
    script: |
      set -euo pipefail
      mkdir -p ~/rcp-server
      umask 077
      : > ~/rcp-server/.env
      IFS=',' read -ra _VARS <<< "${{ env.ENV_LIST }}"
      for _v in "${_VARS[@]}"; do
        printf '%s=%s\n' "$_v" "${!_v}" >> ~/rcp-server/.env
      done
      chmod 0600 ~/rcp-server/.env
```

## 후속 과제

1. **운영 서버 `.env` 검증**: secret 매핑 누락으로 인해 실제 `.env` 파일이 비어 있었는지 확인 필요.
2. **`deploy-ns-proxy.yml` 동일 패턴**: line 69의 `envs: RCP_NS_PROXY_ALLOW_CIDR,RCP_NS_PROXY_ROUTER_ID`도 secret 매핑이 누락됨. 다만 이 값들이 secrets인지 variables인지(혹은 다른 경로로 노출되는지) 별도 확인 후 동일하게 정리할지 결정.
3. 향후 secrets 수가 더 늘어나면 옵션 A(toJSON + prefix 필터링)로의 추가 리팩토링 검토 가능.
