package health

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/config"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(cfg config.Config) *Module {
	service := newService(cfg)
	return &Module{
		Service:    service,
		controller: newController(service),
	}
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.controller.register(r)
}
