package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"retry-proxy/internal/database"
	"retry-proxy/internal/jobs"
	"retry-proxy/internal/routing"
)

type Handler struct {
	repo   *jobs.Repository
	router *routing.Router
	db     *sql.DB
}

func NewHandler(repo *jobs.Repository, router *routing.Router, db *sql.DB) *Handler {
	return &Handler{repo: repo, router: router, db: db}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/_proxy/health" {
		h.health(w, r)
		return
	}
	if strings.HasPrefix(path, "/_proxy/jobs/") {
		h.jobStatus(w, r)
		return
	}

	h.ingest(w, r)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := database.Ping(h.db); err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) jobStatus(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/_proxy/jobs/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}

	job, err := h.repo.GetByID(id)
	if err != nil {
		slog.Error("get job", "id", id, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	type jobResp struct {
		ID           int64      `json:"id"`
		State        string     `json:"state"`
		RetryCount   int        `json:"retry_count"`
		ResponseCode *int       `json:"response_code,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
		CompletedAt  *time.Time `json:"completed_at,omitempty"`
	}

	resp := jobResp{
		ID:           job.ID,
		State:        string(job.State),
		RetryCount:   job.RetryCount,
		ResponseCode: job.ResponseCode,
		CreatedAt:    job.CreatedAt,
		CompletedAt:  job.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	cr := h.router.Match(path)
	if cr == nil {
		http.Error(w, `{"error":"no matching route"}`, http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read body", "error", err)
		http.Error(w, "read body failed", http.StatusInternalServerError)
		return
	}

	headersMap := map[string][]string(r.Header)
	headersJSON, err := json.Marshal(headersMap)
	if err != nil {
		slog.Error("marshal headers", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	job := &jobs.Job{
		RouteName:   cr.Route.Name,
		Method:      r.Method,
		RequestPath: path,
		QueryString: r.URL.RawQuery,
		HeadersJSON: string(headersJSON),
		Body:        body,
		NextRetryAt: now,
		DeadlineAt:  now.Add(maxDurationFromContext(r)),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	id, err := h.repo.Create(job)
	if err != nil {
		slog.Error("persist job", "error", err)
		http.Error(w, "failed to persist request", http.StatusInternalServerError)
		return
	}

	slog.Info("job accepted", "job_id", id, "route", cr.Route.Name, "method", r.Method, "path", path)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted": true,
		"job_id":   id,
	})
}

// MaxDurationKey is exported so main can inject the value.
type MaxDurationKey struct{}

func maxDurationFromContext(r *http.Request) time.Duration {
	if d, ok := r.Context().Value(MaxDurationKey{}).(time.Duration); ok {
		return d
	}
	return 10 * time.Minute
}
