package jobs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"retry-proxy/internal/config"
	"retry-proxy/internal/routing"
	"retry-proxy/internal/upstream"
)

type Worker struct {
	repo         *Repository
	router       *routing.Router
	client       *upstream.Client
	retryCodes   map[int]bool
	maxDuration  time.Duration
	backoff      config.BackoffConfig
	pollInterval time.Duration
}

func NewWorker(
	repo *Repository,
	router *routing.Router,
	client *upstream.Client,
	cfg *config.Config,
) *Worker {
	codes := make(map[int]bool)
	for _, c := range cfg.Retry.RetryStatusCodes {
		codes[c] = true
	}
	return &Worker{
		repo:         repo,
		router:       router,
		client:       client,
		retryCodes:   codes,
		maxDuration:  cfg.Retry.MaxDuration,
		backoff:      cfg.Retry.Backoff,
		pollInterval: cfg.Worker.PollInterval,
	}
}

func (w *Worker) Run(stop <-chan struct{}) {
	for {
		job, err := w.repo.ClaimNext()
		if err != nil {
			slog.Error("claim job", "error", err)
		} else if job != nil {
			w.process(job)
			continue
		}

		select {
		case <-stop:
			return
		case <-time.After(w.pollInterval):
		}
	}
}

func (w *Worker) process(job *Job) {
	log := slog.With("job_id", job.ID, "route", job.RouteName, "attempt", job.RetryCount+1)

	if time.Now().After(job.DeadlineAt) {
		if err := w.repo.MarkExpired(job.ID); err != nil {
			log.Error("mark expired", "error", err)
		}
		log.Info("job expired")
		return
	}

	cr := w.router.Match(job.RequestPath)
	if cr == nil {
		errMsg := "no matching route"
		_ = w.repo.MarkFailed(job.ID, errMsg)
		log.Error("no route", "path", job.RequestPath)
		return
	}

	rewrittenPath := cr.RewritePath(job.RequestPath)
	targetURL := buildURL(cr.Route.Target.BaseURL, rewrittenPath, job.QueryString)

	var headers map[string][]string
	if err := json.Unmarshal([]byte(job.HeadersJSON), &headers); err != nil {
		_ = w.repo.MarkFailed(job.ID, "invalid headers JSON")
		return
	}

	resp, err := w.client.Do(job.Method, targetURL, headers, job.Body)
	if err != nil {
		log.Warn("upstream error", "error", err)
		w.scheduleRetry(job, fmt.Sprintf("network error: %s", err), nil, nil)
		return
	}

	log.Info("upstream response", "status", resp.StatusCode, "url", targetURL)

	if !w.retryCodes[resp.StatusCode] {
		respHeaders, _ := json.Marshal(map[string][]string(resp.Headers))
		if err := w.repo.MarkCompleted(job.ID, resp.StatusCode, respHeaders, resp.Body); err != nil {
			log.Error("mark completed", "error", err)
		}
		return
	}

	retryAfter := parseRetryAfter(resp.Headers.Get("Retry-After"))
	w.scheduleRetry(job, fmt.Sprintf("status %d", resp.StatusCode), &resp.StatusCode, retryAfter)
}

func (w *Worker) scheduleRetry(job *Job, lastErr string, code *int, retryAfterOverride *time.Time) {
	log := slog.With("job_id", job.ID)

	newCount := job.RetryCount + 1
	var nextRetry time.Time

	if retryAfterOverride != nil {
		nextRetry = *retryAfterOverride
	} else {
		nextRetry = time.Now().Add(w.calcBackoff(newCount))
	}

	if nextRetry.After(job.DeadlineAt) {
		if err := w.repo.MarkExpired(job.ID); err != nil {
			log.Error("mark expired", "error", err)
		}
		log.Info("job expired after retry schedule", "error", lastErr)
		return
	}

	if err := w.repo.ScheduleRetry(job.ID, nextRetry, newCount, lastErr, code); err != nil {
		log.Error("schedule retry", "error", err)
	} else {
		log.Info("retry scheduled", "next_retry_at", nextRetry, "error", lastErr)
	}
}

func (w *Worker) calcBackoff(attempt int) time.Duration {
	d := w.backoff.Initial
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > w.backoff.Max {
			d = w.backoff.Max
			break
		}
	}
	return d
}

func parseRetryAfter(header string) *time.Time {
	if header == "" {
		return nil
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil {
		t := time.Now().Add(time.Duration(secs) * time.Second)
		return &t
	}
	formats := []string{http.TimeFormat, time.RFC1123, time.RFC850, time.ANSIC}
	for _, f := range formats {
		if t, err := time.Parse(f, header); err == nil {
			return &t
		}
	}
	return nil
}

func buildURL(baseURL, path, query string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + path
	}
	p, _ := url.Parse(path)
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(p.Path, "/")
	if query != "" {
		u.RawQuery = query
	}
	return u.String()
}
