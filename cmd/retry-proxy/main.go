package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"retry-proxy/internal/api"
	"retry-proxy/internal/config"
	"retry-proxy/internal/database"
	"retry-proxy/internal/jobs"
	"retry-proxy/internal/routing"
	"retry-proxy/internal/upstream"
)


func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := jobs.NewRepository(db)

	if err := repo.RecoverProcessing(); err != nil {
		slog.Error("recover processing jobs", "error", err)
		os.Exit(1)
	}

	router, err := routing.NewRouter(cfg.Routes)
	if err != nil {
		slog.Error("build router", "error", err)
		os.Exit(1)
	}

	client := upstream.NewClient(cfg.HTTP.Timeout)
	worker := jobs.NewWorker(repo, router, client, cfg)

	stop := make(chan struct{})
	for i := 0; i < cfg.Worker.Concurrency; i++ {
		go worker.Run(stop)
	}
	go jobs.RunCleaner(repo, cfg.Cleanup.MaxAge, cfg.Cleanup.PurgeAge, cfg.Cleanup.Interval, stop)

	handler := api.NewHandler(repo, router, db)
	mux := http.NewServeMux()
	mux.Handle("/", withMaxDuration(handler, cfg.Retry.MaxDuration))

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
	}

	go func() {
		slog.Info("server starting", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	close(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func withMaxDuration(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), api.MaxDurationKey{}, d)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
