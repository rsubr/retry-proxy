package jobs

import (
	"log/slog"
	"time"
)

func RunCleaner(repo *Repository, maxAge, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cutoff := time.Now().UTC().Add(-maxAge)
			n, err := repo.ExpireOldQueued(cutoff)
			if err != nil {
				slog.Error("cleanup old queued jobs", "error", err)
			} else if n > 0 {
				slog.Info("expired old queued jobs", "count", n, "older_than", cutoff)
			}
		}
	}
}
