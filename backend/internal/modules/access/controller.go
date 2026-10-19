package access

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/apiresponse"
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
	r.GET("/admin/permissions", guard.RequirePermission(rbac.PermissionGroupsView), c.listPermissions)
	r.GET("/admin/groups", guard.RequirePermission(rbac.PermissionGroupsView), c.listGroups)
	r.POST("/admin/groups", guard.RequirePermission(rbac.PermissionGroupsEdit), c.createGroup)
	r.PATCH("/admin/groups/:id", guard.RequirePermission(rbac.PermissionGroupsEdit), c.updateGroup)
	r.DELETE("/admin/groups/:id", guard.RequirePermission(rbac.PermissionGroupsEdit), c.deleteGroup)
	r.GET("/admin/users", guard.RequirePermission(rbac.PermissionUsersView), c.listUsers)
	r.GET("/admin/users/:id", guard.RequirePermission(rbac.PermissionUsersView), c.getUser)
	r.PATCH("/admin/users/:id", guard.RequirePermission(rbac.PermissionUsersEdit), c.updateUser)
	r.DELETE("/admin/users", guard.RequirePermission(rbac.PermissionUsersEdit), c.deleteUsers)
	r.DELETE("/admin/users/:id", guard.RequirePermission(rbac.PermissionUsersEdit), c.deleteUser)
}

func (c *controller) listPermissions(ctx *gin.Context) {
	catalog := rbac.Catalog()
	items := make([]permissionResponse, 0, len(catalog))
	for _, permission := range catalog {
		items = append(items, permissionResponse{Key: permission.Key, Name: permission.Name})
	}
	ctx.JSON(http.StatusOK, apiresponse.ItemList[permissionResponse]{Items: items})
}

func (c *controller) listGroups(ctx *gin.Context) {
	groups, err := c.service.ListGroups(ctx.Request.Context())
	if err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.ItemList[Group]{Items: groups})
}

func (c *controller) createGroup(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	var request groupRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || request.Name == nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	permissions := []string{}
	if request.Permissions != nil {
		permissions = *request.Permissions
	}
	group, err := c.service.CreateGroup(ctx.Request.Context(), operator, *request.Name, permissions)
	if err != nil {
		apierror.Write(ctx, err)
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
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	if request.Name == nil && request.Permissions == nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	group, err := c.service.UpdateGroup(ctx.Request.Context(), operator, ctx.Param("id"), UpdateGroupInput{
		Name:        request.Name,
		Permissions: request.Permissions,
	})
	if err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, group)
}

func (c *controller) deleteGroup(ctx *gin.Context) {
	if err := c.service.DeleteGroup(ctx.Request.Context(), ctx.Param("id")); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.Deleted{ID: ctx.Param("id"), Deleted: true})
}

func (c *controller) listUsers(ctx *gin.Context) {
	users, err := c.service.ListUsers(ctx.Request.Context())
	if err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.ItemList[UserSummary]{Items: users})
}

func (c *controller) getUser(ctx *gin.Context) {
	user, err := c.service.GetUser(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		apierror.Write(ctx, err)
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
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	if request.GroupID == nil && request.ExtraPermissions == nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	if request.GroupID != nil {
		if _, err := uuid.Parse(*request.GroupID); err != nil {
			apierror.Write(ctx, apierror.ErrInvalidRequest)
			return
		}
	}
	user, err := c.service.UpdateUser(ctx.Request.Context(), operator, ctx.Param("id"), UpdateUserInput{
		GroupID:          request.GroupID,
		ExtraPermissions: request.ExtraPermissions,
	})
	if err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, user)
}

func (c *controller) deleteUsers(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	var request userBatchRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	if err := c.service.DeleteUsers(ctx.Request.Context(), operator, request.IDs); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.DeletedBatch{IDs: request.IDs, Deleted: true})
}

func (c *controller) deleteUser(ctx *gin.Context) {
	operator, ok := operatorFrom(ctx)
	if !ok {
		return
	}
	if err := c.service.DeleteUser(ctx.Request.Context(), operator, ctx.Param("id")); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.Deleted{ID: ctx.Param("id"), Deleted: true})
}

func operatorFrom(ctx *gin.Context) (guard.Identity, bool) {
	identity, ok := guard.IdentityFrom(ctx)
	if !ok {
		apierror.Write(ctx, apierror.ErrInternal)
		return guard.Identity{}, false
	}
	return identity, true
}
