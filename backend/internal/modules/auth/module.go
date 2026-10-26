package auth

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/database"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/passwordreset"
	"website/backend/internal/modules/registration"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
	"website/backend/internal/modules/twofactor"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database, sessions *session.Service, settingsService *settings.Service, loginService *login.Service, registrationService *registration.Service, twoFactorService *twofactor.Service, passwordResetService *passwordreset.Service, mailer mail.Mailer) *Module {
	service := newService(db, sessions, settingsService, mailer)
	return &Module{
		Service:    service,
		controller: newController(service, loginService, registrationService, twoFactorService, passwordResetService),
	}
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.controller.register(r)
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
