package health

import (
	"context"
	"time"

	"website/backend/internal/config"
)

const pingTimeout = 2 * time.Second

type pinger interface {
	Ping(ctx context.Context) error
}

type Service struct {
	cfg       config.Config
	db        pinger
	startedAt time.Time
}

func newService(cfg config.Config, db pinger) *Service {
	return &Service{
		cfg:       cfg,
		db:        db,
		startedAt: time.Now(),
	}
}

func (s *Service) Check(ctx context.Context) HealthResponse {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	status := HealthResponse{
		Status:      "ok",
		Service:     s.cfg.ServiceName,
		Version:     s.cfg.Version,
		Environment: s.cfg.Environment,
		Database:    "ok",
		Uptime:      time.Since(s.startedAt).Round(time.Second).String(),
		Time:        time.Now().Format(time.RFC3339),
	}

	if err := s.db.Ping(pingCtx); err != nil {
		status.Status = "degraded"
		status.Database = "error"
	}

	return status
}
