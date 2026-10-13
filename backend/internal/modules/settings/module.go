package settings

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"website/backend/internal/database"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database) (*Module, error) {
	service, err := newServiceWith(db, builtinRegistry, slog.Default())
	if err != nil {
		return nil, err
	}
	return &Module{
		Service:    service,
		controller: newController(service),
	}, nil
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.controller.register(r)
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}
