package health

import (
	"time"

	"website/backend/internal/config"
)

type Service struct {
	cfg       config.Config
	startedAt time.Time
}

func newService(cfg config.Config) *Service {
	return &Service{cfg: cfg, startedAt: time.Now()}
}

func (s *Service) Check() Status {
	return Status{
		Status:      "ok",
		Service:     s.cfg.ServiceName,
		Version:     s.cfg.Version,
		Environment: s.cfg.Environment,
		Uptime:      time.Since(s.startedAt).Round(time.Second).String(),
		Time:        time.Now().Format(time.RFC3339),
	}
}
