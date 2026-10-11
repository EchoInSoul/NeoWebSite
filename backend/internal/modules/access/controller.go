package access

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

type errorResponse struct {
	Error string `json:"error"`
}

type itemList[T any] struct {
	Items []T `json:"items"`
}

type permissionResponse struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type groupRequest struct {
	Name        *string   `json:"name"`
	Permissions *[]string `json:"permissions"`
}

type userRequest struct {
	GroupID          *string   `json:"group_id"`
	ExtraPermissions *[]string `json:"extra_permissions"`
}

type controller struct {
	service *Service
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.GET("/admin/permissions", guard.RequirePermission(rbac.PermissionGroupsView), c.listPermissions)
	r.GET("/admin/groups", guard.RequirePermission(rbac.PermissionGroupsView), c.listGroups)
	r.POST("/admin/groups", guard.RequirePermission(rbac.PermissionGroupsEdit), c.createGroup)
	r.PATCH("/admin/groups/:id", guard.RequirePermission(rbac.PermissionGroupsEdit), c.updateGroup)
	r.DELETE("/admin/groups/:id", guard.RequirePermission(rbac.PermissionGroupsEdit), c.deleteGroup)
	r.GET("/admin/users", guard.RequirePermission(rbac.PermissionUsersView), c.listUsers)
	r.GET("/admin/users/:id", guard.RequirePermission(rbac.PermissionUsersView), c.getUser)
	r.PATCH("/admin/users/:id", guard.RequirePermission(rbac.PermissionUsersEdit), c.updateUser)
}

func (c *controller) listPermissions(ctx *gin.Context) {
	catalog := rbac.Catalog()
	items := make([]permissionResponse, 0, len(catalog))
	for _, permission := range catalog {
		items = append(items, permissionResponse{Key: permission.Key, Name: permission.Name})
	}
	ctx.JSON(http.StatusOK, itemList[permissionResponse]{Items: items})
}

func (c *controller) listGroups(ctx *gin.Context) {
	groups, err := c.service.ListGroups(ctx.Request.Context())
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, itemList[Group]{Items: groups})
}

func (c *controller) createGroup(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	var request groupRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || request.Name == nil {
		writeInvalidRequest(ctx)
		return
	}
	permissions := []string{}
	if request.Permissions != nil {
		permissions = *request.Permissions
	}
	group, err := c.service.CreateGroup(ctx.Request.Context(), operator, *request.Name, permissions)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, group)
}

func (c *controller) updateGroup(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	var request groupRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeInvalidRequest(ctx)
		return
	}
	if request.Name == nil && request.Permissions == nil {
		writeInvalidRequest(ctx)
		return
	}
	group, err := c.service.UpdateGroup(ctx.Request.Context(), operator, ctx.Param("id"), UpdateGroupInput{
		Name:        request.Name,
		Permissions: request.Permissions,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, group)
}

func (c *controller) deleteGroup(ctx *gin.Context) {
	if err := c.service.DeleteGroup(ctx.Request.Context(), ctx.Param("id")); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *controller) listUsers(ctx *gin.Context) {
	users, err := c.service.ListUsers(ctx.Request.Context())
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, itemList[UserSummary]{Items: users})
}

func (c *controller) getUser(ctx *gin.Context) {
	user, err := c.service.GetUser(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, user)
}

func (c *controller) updateUser(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	var request userRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeInvalidRequest(ctx)
		return
	}
	if request.GroupID == nil && request.ExtraPermissions == nil {
		writeInvalidRequest(ctx)
		return
	}
	if request.GroupID != nil {
		if _, err := uuid.Parse(*request.GroupID); err != nil {
			writeInvalidRequest(ctx)
			return
		}
	}
	user, err := c.service.UpdateUser(ctx.Request.Context(), operator, ctx.Param("id"), UpdateUserInput{
		GroupID:          request.GroupID,
		ExtraPermissions: request.ExtraPermissions,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, user)
}

func operatorFrom(ctx *gin.Context) (guard.Identity, bool) {
	identity, ok := guard.IdentityFrom(ctx)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
		return guard.Identity{}, false
	}
	return identity, true
}

func writeInvalidRequest(ctx *gin.Context) {
	ctx.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
}

func writeError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		ctx.JSON(http.StatusNotFound, errorResponse{Error: "not_found"})
	case errors.Is(err, ErrInvalidName):
		ctx.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_request"})
	case errors.Is(err, ErrInvalidPermission):
		ctx.JSON(http.StatusBadRequest, errorResponse{Error: "invalid_permission"})
	case errors.Is(err, ErrGroupNameTaken):
		ctx.JSON(http.StatusConflict, errorResponse{Error: "group_name_taken"})
	case errors.Is(err, ErrGroupNotEmpty):
		ctx.JSON(http.StatusConflict, errorResponse{Error: "group_not_empty"})
	case errors.Is(err, ErrLastOwner):
		ctx.JSON(http.StatusConflict, errorResponse{Error: "last_owner"})
	case errors.Is(err, ErrGrantForbidden):
		ctx.JSON(http.StatusForbidden, errorResponse{Error: "grant_forbidden"})
	case errors.Is(err, ErrOwnerProtected):
		ctx.JSON(http.StatusForbidden, errorResponse{Error: "owner_protected"})
	case errors.Is(err, ErrCannotModifySelf):
		ctx.JSON(http.StatusForbidden, errorResponse{Error: "cannot_modify_self"})
	case errors.Is(err, ErrPresetGroupLocked):
		ctx.JSON(http.StatusForbidden, errorResponse{Error: "preset_group_locked"})
	default:
		ctx.JSON(http.StatusInternalServerError, errorResponse{Error: "internal_error"})
	}
}
