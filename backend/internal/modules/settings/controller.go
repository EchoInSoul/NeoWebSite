package settings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

type controller struct {
	service *Service
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.GET("/admin/settings/:group", guard.RequirePermission(rbac.PermissionSettingsView), c.getGroup)
}

func (c *controller) getGroup(ctx *gin.Context) {
	values, err := c.service.GetGroup(ctx.Request.Context(), ctx.Param("group"))
	if err != nil {
		writeSettingError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, values)
}

func writeSettingError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnknownGroup):
		apierror.Write(ctx, apierror.ErrNotFound)
	case errors.Is(err, ErrUnknownItem), errors.Is(err, ErrTypeMismatch), errors.Is(err, ErrMethodMismatch):
		apierror.Write(ctx, apierror.ErrInvalidRequest)
	default:
		apierror.Write(ctx, err)
	}
}
