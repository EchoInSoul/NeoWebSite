package media

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/database"
)

type Module struct {
	Service *Service

	controller *controller
}

func New(db *database.Database, root string) (*Module, error) {
	service, err := newService(db, root, allowAll{})
	if err != nil {
		return nil, err
	}
	return &Module{Service: service, controller: newController(service, service.Root())}, nil
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.controller.registerProtected(r)
}

func (m *Module) RegisterStorage(engine *gin.Engine) {
	m.controller.registerStorage(engine)
}
