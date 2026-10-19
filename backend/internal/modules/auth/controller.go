package auth

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/apiresponse"
	"website/backend/internal/common/guard"
)

type controller struct {
	service *Service
}

func newController(service *Service) *controller {
	return &controller{service: service}
}

func (c *controller) register(r *gin.RouterGroup) {
	r.POST("/users", c.signup)
	r.POST("/sessions", c.login)
	r.POST("/sessions/current/verify", c.verifySecondFactor)
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.DELETE("/sessions/current", c.logout)
	r.GET("/users/me", c.me)
	r.PATCH("/users/me", c.updateProfile)
	r.GET("/users/me/sessions", c.listSessions)
	r.DELETE("/users/me/sessions/:id", c.revokeSession)
	r.POST("/users/me/password", c.changePassword)
	r.POST("/users/me/2fa/setup", c.setupTwoFactor)
	r.POST("/users/me/2fa/enable", c.enableTwoFactor)
	r.POST("/users/me/2fa/disable", c.disableTwoFactor)
	r.POST("/users/me/2fa/recovery-codes", c.regenerateRecoveryCodes)
}

func (c *controller) login(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	var req LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Identifier == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	user, err := c.service.Login(ctx.Request.Context(), req.Identifier, req.Password, requestSessionMeta(ctx))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname, Bio: user.Bio})
}

func (c *controller) signup(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	var req RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Nickname == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	user, err := c.service.Register(ctx.Request.Context(), req.Email, req.Nickname, req.Password, requestSessionMeta(ctx))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, UserResponse{Email: user.Email, Nickname: user.Nickname, Bio: user.Bio})
}

func (c *controller) logout(ctx *gin.Context) {
	if err := c.service.Logout(ctx.Request.Context()); err != nil {
		apierror.Write(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, apiresponse.Empty{})
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
		Bio:      user.Bio,
		Group: GroupResponse{
			ID:        identity.GroupID,
			SystemKey: systemKey,
			Name:      identity.GroupName,
		},
		Permissions:      permissions,
		TwoFactorEnabled: user.TwoFactorEnabled,
	})
}

func (c *controller) updateProfile(ctx *gin.Context) {
	var req ProfileUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	user, err := c.service.UpdateProfile(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Nickname, req.Email, req.Bio)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname, Bio: user.Bio})
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

	ctx.JSON(http.StatusOK, apiresponse.Empty{})
}

func (c *controller) setupTwoFactor(ctx *gin.Context) {
	var req TwoFactorSetupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	resp, err := c.service.SetupTOTP(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) enableTwoFactor(ctx *gin.Context) {
	var req TwoFactorEnableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Code == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	resp, err := c.service.EnableTOTP(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Code, req.Password, currentToken)
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

	ctx.JSON(http.StatusOK, apiresponse.Empty{})
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

func (c *controller) verifySecondFactor(ctx *gin.Context) {
	var req TwoFactorCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Code == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "code"))
		return
	}

	user, err := c.service.VerifySecondFactor(ctx.Request.Context(), req.Code, requestSessionMeta(ctx))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, UserResponse{Email: user.Email, Nickname: user.Nickname, Bio: user.Bio})
}

func (c *controller) listSessions(ctx *gin.Context) {
	sessions, err := c.service.ListSessions(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	items := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, SessionResponse{
			ID:         session.ID,
			Current:    session.Current,
			UserAgent:  session.UserAgent,
			IP:         session.IP,
			CreatedAt:  session.CreatedAt,
			LastSeenAt: session.LastSeenAt,
			ExpiresAt:  session.ExpiresAt,
		})
	}

	ctx.JSON(http.StatusOK, apiresponse.ItemList[SessionResponse]{Items: items})
}

func (c *controller) revokeSession(ctx *gin.Context) {
	if err := c.service.RevokeSession(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), ctx.Param("id")); err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, apiresponse.Deleted{ID: ctx.Param("id"), Deleted: true})
}

const (
	maxUserAgentLength = 512
	maxIPLength        = 64
)

func requestSessionMeta(ctx *gin.Context) SessionMeta {
	return SessionMeta{
		UserAgent: truncateHeader(ctx.GetHeader("User-Agent"), maxUserAgentLength),
		IP:        truncateHeader(ctx.ClientIP(), maxIPLength),
	}
}

func truncateHeader(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func requireJSONRequest(c *gin.Context) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return false
	}

	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	expected := c.GetHeader("X-Forwarded-Host")
	if idx := strings.IndexByte(expected, ','); idx >= 0 {
		expected = expected[:idx]
	}
	expected = strings.TrimSpace(expected)
	if expected == "" {
		expected = c.Request.Host
	}

	return strings.EqualFold(parsed.Host, expected)
}
