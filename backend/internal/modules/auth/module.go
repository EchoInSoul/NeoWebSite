package auth

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/registration"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database, sessions *session.Service, totpKey []byte, settingsService *settings.Service, limiter *ratelimit.Limiter, loginService *login.Service, registrationService *registration.Service) *Module {
	service := newService(db, sessions, totpKey, settingsService, limiter, defaultVerifyPolicies())
	return &Module{
		Service:    service,
		controller: newController(service, loginService, registrationService),
	}
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.controller.register(r)
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
