package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   string         `yaml:"listen"`
	Database DatabaseConfig `yaml:"database"`
	Worker   WorkerConfig   `yaml:"worker"`
	HTTP     HTTPConfig     `yaml:"http"`
	Retry    RetryConfig    `yaml:"retry"`
	Cleanup  CleanupConfig  `yaml:"cleanup"`
	Routes   []Route        `yaml:"routes"`
}

type CleanupConfig struct {
	MaxAge   time.Duration `yaml:"max_age"`
	PurgeAge time.Duration `yaml:"purge_age"`
	Interval time.Duration `yaml:"interval"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type WorkerConfig struct {
	Concurrency  int           `yaml:"concurrency"`
	PollInterval time.Duration `yaml:"poll_interval"`
}

type HTTPConfig struct {
	Timeout time.Duration `yaml:"timeout"`
}

type RetryConfig struct {
	MaxDuration       time.Duration `yaml:"max_duration"`
	RetryStatusCodes  []int         `yaml:"retry_status_codes"`
	Backoff           BackoffConfig `yaml:"backoff"`
}

type BackoffConfig struct {
	Strategy string        `yaml:"strategy"`
	Initial  time.Duration `yaml:"initial"`
	Max      time.Duration `yaml:"max"`
}

type Route struct {
	Name    string       `yaml:"name"`
	Match   RouteMatch   `yaml:"match"`
	Rewrite RouteRewrite `yaml:"rewrite"`
	Target  RouteTarget  `yaml:"target"`
}

type RouteMatch struct {
	Prefix string `yaml:"prefix"`
	Regex  string `yaml:"regex"`
}

type RouteRewrite struct {
	StripPrefix bool          `yaml:"strip_prefix"`
	Regex       []RegexRewrite `yaml:"regex"`
}

type RegexRewrite struct {
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
}

type RouteTarget struct {
	BaseURL string `yaml:"base_url"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./queue.db"
	}
	if cfg.Worker.Concurrency == 0 {
		cfg.Worker.Concurrency = 10
	}
	if cfg.Worker.PollInterval == 0 {
		cfg.Worker.PollInterval = 5 * time.Second
	}
	if cfg.HTTP.Timeout == 0 {
		cfg.HTTP.Timeout = 30 * time.Second
	}
	if cfg.Retry.MaxDuration == 0 {
		cfg.Retry.MaxDuration = 10 * time.Minute
	}
	if len(cfg.Retry.RetryStatusCodes) == 0 {
		cfg.Retry.RetryStatusCodes = []int{429, 500, 502, 503, 504}
	}
	if cfg.Retry.Backoff.Strategy == "" {
		cfg.Retry.Backoff.Strategy = "exponential"
	}
	if cfg.Retry.Backoff.Initial == 0 {
		cfg.Retry.Backoff.Initial = 5 * time.Second
	}
	if cfg.Retry.Backoff.Max == 0 {
		cfg.Retry.Backoff.Max = 60 * time.Second
	}
	if cfg.Cleanup.MaxAge == 0 {
		cfg.Cleanup.MaxAge = 7 * 24 * time.Hour
	}
	if cfg.Cleanup.PurgeAge == 0 {
		cfg.Cleanup.PurgeAge = 7 * 24 * time.Hour
	}
	if cfg.Cleanup.Interval == 0 {
		cfg.Cleanup.Interval = 1 * time.Hour
	}
}
