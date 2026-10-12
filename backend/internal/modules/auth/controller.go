package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
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
	r.POST("/auth/register", c.signup)
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.POST("/auth/logout", c.logout)
	r.GET("/auth/me", c.me)
	r.POST("/auth/password", c.changePassword)
	r.POST("/auth/2fa/setup", c.setupTwoFactor)
	r.POST("/auth/2fa/enable", c.enableTwoFactor)
	r.POST("/auth/2fa/disable", c.disableTwoFactor)
	r.POST("/auth/2fa/recovery-codes", c.regenerateRecoveryCodes)
}

func (c *controller) login(ctx *gin.Context) {
	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Identifier == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	user, err := c.service.Login(ctx.Request.Context(), req.Identifier, req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname})
}

func (c *controller) signup(ctx *gin.Context) {
	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Nickname == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	user, err := c.service.Register(ctx.Request.Context(), req.Email, req.Nickname, req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, UserResponse{Email: user.Email, Nickname: user.Nickname})
}

func (c *controller) logout(ctx *gin.Context) {
	if err := c.service.Logout(ctx.Request.Context()); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{})
}

func (c *controller) me(ctx *gin.Context) {
	identity, ok := guard.IdentityFrom(ctx)
	if !ok {
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}

	user, err := c.service.CurrentUser(ctx.Request.Context(), identity.UserID)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	permissions := identity.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	var systemKey *string
	if identity.GroupKey != "" {
		systemKey = &identity.GroupKey
	}
	ctx.JSON(http.StatusOK, MeResponse{
		Email:    user.Email,
		Nickname: user.Nickname,
		Group: GroupResponse{
			ID:        identity.GroupID,
			SystemKey: systemKey,
			Name:      identity.GroupName,
		},
		Permissions: permissions,
	})
}

func (c *controller) changePassword(ctx *gin.Context) {
	var req PasswordChangeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.CurrentPassword == "" || req.NewPassword == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	userID := ctx.GetString(guard.SessionUserIDKey)
	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	err = c.service.ChangePassword(ctx.Request.Context(), userID, currentToken, req.CurrentPassword, req.NewPassword)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{})
}

func (c *controller) setupTwoFactor(ctx *gin.Context) {
	resp, err := c.service.SetupTOTP(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) enableTwoFactor(ctx *gin.Context) {
	var req TwoFactorEnableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Code == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	resp, err := c.service.EnableTOTP(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Code, currentToken)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) disableTwoFactor(ctx *gin.Context) {
	var req TwoFactorDisableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	err = c.service.DisableTOTP(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password, currentToken)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{})
}

func (c *controller) regenerateRecoveryCodes(ctx *gin.Context) {
	var req TwoFactorDisableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	resp, err := c.service.RegenerateRecoveryCodes(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}
