package jobs

import (
	"database/sql"
	"fmt"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(j *Job) (int64, error) {
	res, err := r.db.Exec(`
		INSERT INTO jobs (
			route_name, method, request_path, query_string,
			headers_json, body, state, retry_count,
			next_retry_at, deadline_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.RouteName, j.Method, j.RequestPath, j.QueryString,
		j.HeadersJSON, j.Body, string(StateQueued), 0,
		j.NextRetryAt, j.DeadlineAt, j.CreatedAt, j.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("insert job: %w", err)
	}
	return res.LastInsertId()
}

func (r *Repository) GetByID(id int64) (*Job, error) {
	row := r.db.QueryRow(`
		SELECT id, route_name, method, request_path, query_string,
		       headers_json, body, state, retry_count,
		       next_retry_at, deadline_at, response_code,
		       response_headers_json, response_body, last_error,
		       created_at, updated_at, completed_at
		FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func (r *Repository) ClaimNext() (*Job, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id FROM jobs
		WHERE state = 'queued' AND next_retry_at <= ?
		ORDER BY next_retry_at ASC
		LIMIT 1`, time.Now().UTC())

	var id int64
	if err := row.Scan(&id); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	res, err := tx.Exec(`
		UPDATE jobs SET state = 'processing', updated_at = ?
		WHERE id = ? AND state = 'queued'`,
		time.Now().UTC(), id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(id)
}

func (r *Repository) MarkCompleted(id int64, code int, respHeaders, respBody []byte) error {
	now := time.Now().UTC()
	codePtr := code
	headersStr := string(respHeaders)
	_, err := r.db.Exec(`
		UPDATE jobs
		SET state='completed', response_code=?, response_headers_json=?,
		    response_body=?, completed_at=?, updated_at=?
		WHERE id=?`,
		codePtr, headersStr, respBody, now, now, id)
	return err
}

func (r *Repository) MarkFailed(id int64, lastErr string) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(`
		UPDATE jobs SET state='failed', last_error=?, updated_at=?
		WHERE id=?`, lastErr, now, id)
	return err
}

func (r *Repository) MarkExpired(id int64) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(`
		UPDATE jobs SET state='expired', updated_at=?
		WHERE id=?`, now, id)
	return err
}

func (r *Repository) ScheduleRetry(id int64, nextRetry time.Time, retryCount int, lastErr string, code *int) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(`
		UPDATE jobs
		SET state='queued', next_retry_at=?, retry_count=?,
		    last_error=?, response_code=?, updated_at=?
		WHERE id=?`,
		nextRetry, retryCount, lastErr, code, now, id)
	return err
}

func (r *Repository) RecoverProcessing() error {
	_, err := r.db.Exec(`
		UPDATE jobs SET state='queued'
		WHERE state='processing'`)
	return err
}

func scanJob(row *sql.Row) (*Job, error) {
	var j Job
	var state string
	var nextRetry, deadline, created, updated string
	var completedAt sql.NullString

	err := row.Scan(
		&j.ID, &j.RouteName, &j.Method, &j.RequestPath, &j.QueryString,
		&j.HeadersJSON, &j.Body, &state, &j.RetryCount,
		&nextRetry, &deadline, &j.ResponseCode,
		&j.ResponseHeadersJSON, &j.ResponseBody, &j.LastError,
		&created, &updated, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	j.State = State(state)
	j.NextRetryAt, _ = time.Parse(time.RFC3339Nano, nextRetry)
	j.DeadlineAt, _ = time.Parse(time.RFC3339Nano, deadline)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		j.CompletedAt = &t
	}

	return &j, nil
}
