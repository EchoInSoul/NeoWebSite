package captcha

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/modules/settings"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(settingsService *settings.Service) *Module {
	service := NewService(settingsService, newTurnstileVerifier(turnstileSiteverifyURL, turnstileTimeout))
	return &Module{
		Service:    service,
		controller: newController(service),
	}
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
