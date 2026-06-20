package jobs

import (
	"log/slog"
	"time"
)

func RunCleaner(repo *Repository, maxAge, purgeAge, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now().UTC()

			n, err := repo.ExpireOldQueued(now.Add(-maxAge))
			if err != nil {
				slog.Error("expire old queued jobs", "error", err)
			} else if n > 0 {
				slog.Info("expired old queued jobs", "count", n)
			}

			n, err = repo.PurgeOldJobs(now.Add(-purgeAge))
			if err != nil {
				slog.Error("purge old jobs", "error", err)
			} else if n > 0 {
				slog.Info("purged old jobs", "count", n)
			}
		}
	}
}
