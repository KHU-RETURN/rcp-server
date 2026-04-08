package database

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

func NewSQLiteConnection() (*sql.DB, error) {
	// ":memory:"를 사용하면 서버 종료 시 데이터가 사라집니다.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	// 커넥션 풀 설정 (SQLite는 파일 기반이라 동시성 제한이 필요할 수 있음)
	db.SetMaxOpenConns(1)

	// modernc.org/sqlite는 DSN에 _fk=1을 지원하지 않으므로 PRAGMA로 직접 활성화합니다.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, err
	}

	return db, nil
}
