package mail

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"website/backend/internal/config"
)

type Module struct {
	Mailer Mailer

	controller *controller
}

func New(cfg config.Config, settings Settings) *Module {
	logger := slog.Default()
	m := newMailer(newTransport(cfg, settings, logger), logger)
	go m.run()
	return &Module{
		Mailer:     m,
		controller: newController(m),
	}
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
