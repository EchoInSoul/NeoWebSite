package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/guard"
)

type controller struct {
	service *Service
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) register(r *gin.RouterGroup) {
	r.POST("/auth/login", c.login)
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.POST("/auth/logout", c.logout)
	r.GET("/auth/me", c.me)
}

func (c *controller) login(ctx *gin.Context) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Identifier == "" || req.Password == "" {
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_request"})
		return
	}

	user, err := c.service.Login(ctx.Request.Context(), req.Identifier, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			ctx.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid_credentials"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal_error"})
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname})
}

func (c *controller) logout(ctx *gin.Context) {
	if err := c.service.Logout(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal_error"})
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *controller) me(ctx *gin.Context) {
	userID := ctx.GetString(guard.SessionUserIDKey)

	user, err := c.service.CurrentUser(ctx.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			ctx.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal_error"})
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname})
}
