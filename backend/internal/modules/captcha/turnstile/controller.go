package turnstile

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) RegisterProtected(r *gin.RouterGroup) {
	r.POST("/admin/captcha/verifications", guard.RequirePermission(rbac.PermissionCaptchaEdit), c.verify)
}

func (c *Controller) verify(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	var req VerificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.SecretKey == "" || req.Token == "" {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	if err := c.service.VerifyConfig(ctx.Request.Context(), req.SecretKey, req.Token, clientIP(ctx)); err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, VerificationResponse{Verified: true})
}

func clientIP(ctx *gin.Context) string {
	return ctx.RemoteIP()
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
