package settings

import (
	"encoding/json"
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

type groupView struct {
	Group  string         `json:"group"`
	Values map[string]any `json:"values"`
}

type publicSettingsView struct {
	Items []groupView `json:"items"`
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.GET("/admin/settings/:group", requireGroupPermission(rbac.PermissionSettingsView, rbac.PermissionRegisterView), c.getGroup)
	r.PATCH("/admin/settings/:group", requireGroupPermission(rbac.PermissionSettingsEdit, rbac.PermissionRegisterEdit), c.patchGroup)
}

func requireGroupPermission(settingsPermission, registerPermission string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		permission := settingsPermission
		switch ctx.Param("group") {
		case "register", "auth":
			permission = registerPermission
		}
		guard.RequirePermission(permission)(ctx)
	}
}

func (c *controller) register(r *gin.RouterGroup) {
	r.GET("/public/settings", c.getPublic)
}

func (c *controller) getPublic(ctx *gin.Context) {
	groups, err := c.service.PublicSettings(ctx.Request.Context())
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	items := make([]groupView, 0, len(groups))
	for _, group := range groups {
		items = append(items, groupView{Group: group.Group, Values: group.Values})
	}
	ctx.JSON(http.StatusOK, publicSettingsView{Items: items})
}

func (c *controller) getGroup(ctx *gin.Context) {
	group := ctx.Param("group")
	values, err := c.service.GetGroup(ctx.Request.Context(), group)
	if err != nil {
		writeSettingError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, groupView{Group: group, Values: values})
}

func (c *controller) patchGroup(ctx *gin.Context) {
	var raw map[string]json.RawMessage
	if err := ctx.ShouldBindJSON(&raw); err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	group := ctx.Param("group")
	values, err := c.service.PatchGroup(ctx.Request.Context(), group, raw)
	if err != nil {
		writeSettingError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, groupView{Group: group, Values: values})
}

func writeSettingError(ctx *gin.Context, err error) {
	var itemErr *ItemError
	if errors.As(err, &itemErr) {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, itemErr.Item))
		return
	}

	switch {
	case errors.Is(err, ErrUnknownGroup):
		apierror.Write(ctx, apierror.ErrNotFound)
	case errors.Is(err, ErrUnknownItem), errors.Is(err, ErrTypeMismatch), errors.Is(err, ErrMethodMismatch):
		apierror.Write(ctx, apierror.ErrInvalidRequest)
	default:
		apierror.Write(ctx, err)
	}
}
