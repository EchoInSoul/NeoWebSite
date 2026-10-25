package builtin

import (
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Register(r *gin.RouterGroup) {
	r.POST("/captcha/challenges", c.createChallenge)
	r.POST("/captcha/challenges/:id/solutions", c.solveChallenge)
}

func (c *Controller) createChallenge(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	var req CreateChallengeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	response, err := c.service.Challenge(ctx.Request.Context(), req.Scope, clientIP(ctx))
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, response)
}

func (c *Controller) solveChallenge(ctx *gin.Context) {
	if !requireJSONRequest(ctx) {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "id"))
		return
	}

	var req SolveChallengeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}
	if req.X == nil || *req.X < 0 {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "x"))
		return
	}
	if req.Y == nil || *req.Y < 0 {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "y"))
		return
	}

	response, err := c.service.Solve(ctx.Request.Context(), id, *req.X, *req.Y)
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
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
