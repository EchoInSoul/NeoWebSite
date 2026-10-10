package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type controller struct {
	service *Service
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) register(r *gin.RouterGroup) {
	r.GET("/healthy", c.check)
}

func (c *controller) check(ctx *gin.Context) {
	status := c.service.Check(ctx.Request.Context())

	code := http.StatusOK
	if status.Status != "ok" {
		code = http.StatusServiceUnavailable
	}

	ctx.JSON(code, status)
}
