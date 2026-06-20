package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;
		PRAGMA foreign_keys=ON;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragmas: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id                   INTEGER PRIMARY KEY AUTOINCREMENT,
			route_name           TEXT NOT NULL,
			method               TEXT NOT NULL,
			request_path         TEXT NOT NULL,
			query_string         TEXT,
			headers_json         TEXT NOT NULL,
			body                 BLOB,
			state                TEXT NOT NULL,
			retry_count          INTEGER NOT NULL DEFAULT 0,
			next_retry_at        DATETIME NOT NULL,
			deadline_at          DATETIME NOT NULL,
			response_code        INTEGER,
			response_headers_json TEXT,
			response_body        BLOB,
			last_error           TEXT,
			created_at           DATETIME NOT NULL,
			updated_at           DATETIME NOT NULL,
			completed_at         DATETIME
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_state_retry
		ON jobs(state, next_retry_at);
	`)
	return err
}

func Ping(db *sql.DB) error {
	return db.Ping()
}
