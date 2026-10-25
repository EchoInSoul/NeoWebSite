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
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/registration"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/twofactor"
)

type controller struct {
	service             *Service
	loginService        *login.Service
	registrationService *registration.Service
	twoFactorService    *twofactor.Service
}

func newController(service *Service, loginService *login.Service, registrationService *registration.Service, twoFactorService *twofactor.Service) *controller {
	return &controller{
		service:             service,
		loginService:        loginService,
		registrationService: registrationService,
		twoFactorService:    twoFactorService,
	}
}

func (c *controller) register(r *gin.RouterGroup) {
	r.POST("/users", c.signup)
	r.POST("/users/email-verifications", c.verifyEmailRegistration)
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
	r.POST("/users/me/2fa/recovery-codes/email", c.emailRecoveryCodes)
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

	user, err := c.loginService.Login(ctx.Request.Context(), req.Identifier, req.Password, req.CaptchaToken, requestSessionMeta(ctx))
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

	result, err := c.registrationService.Register(ctx.Request.Context(), req.Email, req.Nickname, req.Password, req.CaptchaToken, requestSessionMeta(ctx))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	if result.VerificationPending {
		ctx.JSON(http.StatusAccepted, RegisterPendingResponse{Status: "verification_pending"})
		return
	}

	ctx.JSON(http.StatusCreated, UserResponse{Email: result.User.Email, Nickname: result.User.Nickname, Bio: result.User.Bio})
}

func (c *controller) verifyEmailRegistration(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	var req EmailVerificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Token == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "token"))
		return
	}

	user, err := c.registrationService.VerifyEmail(ctx.Request.Context(), req.Token, requestSessionMeta(ctx))
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
	var req twofactor.TwoFactorSetupRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	resp, err := c.twoFactorService.Setup(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) enableTwoFactor(ctx *gin.Context) {
	var req twofactor.TwoFactorEnableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Code == "" || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	resp, err := c.twoFactorService.Enable(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Code, req.Password, currentToken)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) disableTwoFactor(ctx *gin.Context) {
	var req twofactor.TwoFactorDisableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	currentToken, err := ctx.Cookie("session")
	if err != nil || currentToken == "" {
		apierror.Write(ctx, apierror.ErrUnauthorized)
		return
	}

	err = c.twoFactorService.Disable(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password, currentToken)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, apiresponse.Empty{})
}

func (c *controller) regenerateRecoveryCodes(ctx *gin.Context) {
	var req twofactor.TwoFactorDisableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Password == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	resp, err := c.twoFactorService.RegenerateRecoveryCodes(ctx.Request.Context(), ctx.GetString(guard.SessionUserIDKey), req.Password)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func (c *controller) emailRecoveryCodes(ctx *gin.Context) {
	apierror.Write(ctx, apierror.ErrNotImplemented)
}

func (c *controller) verifySecondFactor(ctx *gin.Context) {
	var req twofactor.TwoFactorCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Code == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "code"))
		return
	}

	user, err := c.twoFactorService.VerifySecondFactor(ctx.Request.Context(), req.Code, requestSessionMeta(ctx))
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
	for _, item := range sessions {
		items = append(items, SessionResponse{
			ID:         item.ID,
			Current:    item.Current,
			UserAgent:  item.UserAgent,
			IP:         item.IP,
			CreatedAt:  item.CreatedAt,
			LastSeenAt: item.LastSeenAt,
			ExpiresAt:  item.ExpiresAt,
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

func requestSessionMeta(ctx *gin.Context) session.Meta {
	return session.Meta{
		UserAgent: truncateHeader(ctx.GetHeader("User-Agent"), maxUserAgentLength),
		IP:        truncateHeader(ctx.RemoteIP(), maxIPLength),
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
