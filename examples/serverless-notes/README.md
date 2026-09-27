# 작은 메모

RCP의 Go 함수와 바인딩된 SQLite 데이터베이스를 사용하는 정적 메모 앱입니다. 함수 키를 가진 사람끼리 메모를 공유합니다. 브라우저는 키를 탭의 메모리에만 보관하고, 저장소나 URL에 넣지 않습니다.

```bash
GOOS=wasip1 GOARCH=wasm go build -o notes.wasm ./examples/serverless-notes
```

RCP 함수 콘솔에서 다음 순서로 준비합니다.

1. **Databases**에서 `notes-db`를 만듭니다.
2. SQL 편집기에서 `CREATE TABLE notes (id TEXT PRIMARY KEY, text TEXT NOT NULL, created_at TEXT NOT NULL)`을 실행합니다.
3. `notes.wasm`을 **WASM** 언어로 등록하고 **Enable data access**를 켭니다. 소스 빌더가 설치된 환경이라면 `main.go`를 **Go** 언어로 바로 올릴 수도 있습니다.
4. 함수의 **Database bindings**에서 `notes-db`를 `DB`라는 이름으로 연결합니다.
5. 함수 전용 키를 발급합니다.

그다음 정적 페이지를 실행합니다.

```bash
python3 -m http.server 8000 --directory examples/serverless-notes
```

브라우저에서 `http://127.0.0.1:8000`을 열고 함수의 HTTP endpoint와 키를 입력합니다. API는 `GET /notes`, `POST /notes`, `DELETE /notes/:id`를 제공합니다. 메모 본문은 최대 500바이트이고, 목록에는 최신 100개가 표시됩니다.

운영 함수를 등록하기 전 화면과 Go 함수의 요청 흐름을 시험하려면 아래 로컬 데모를 실행합니다. 데모는 Go 함수를 실제 프로세스로 실행하고 SQL 바인딩을 메모리 DB로 모사합니다. 기본 입력된 `local-demo` 키로 연결하면 됩니다. 서버를 종료하면 데모 메모가 지워집니다.

```bash
python3 examples/serverless-notes/local_demo.py
```
