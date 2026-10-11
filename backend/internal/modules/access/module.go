package access

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/database"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database) *Module {
	service := newService(db)
	return &Module{
		Service:    service,
		controller: newController(service),
	}
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
