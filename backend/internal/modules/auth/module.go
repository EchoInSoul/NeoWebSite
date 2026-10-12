package auth

import (
	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/database"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database, sessions *scs.SessionManager, totpKey []byte) *Module {
	service := newService(db, sessions, totpKey)
	return &Module{
		Service:    service,
		controller: newController(service),
	}
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.controller.register(r)
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
