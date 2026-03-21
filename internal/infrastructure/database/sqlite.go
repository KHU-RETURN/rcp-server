package database

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func NewSQLiteConnection() (*sql.DB, error) {
	// ":memory:"를 사용하면 서버 종료 시 데이터가 사라집니다.
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}

	// 커넥션 풀 설정 (SQLite는 파일 기반이라 동시성 제한이 필요할 수 있음)
	db.SetMaxOpenConns(1)

	return db, nil
}
