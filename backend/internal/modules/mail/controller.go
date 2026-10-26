package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

const (
	selfCheckSubject      = "邮件配置自检"
	TemplateSignupCode    = "signup_code"
	TemplatePasswordReset = "password_reset"
	exampleTemplate       = "example"
	signupCodeTemplate    = TemplateSignupCode
	passwordResetTemplate = TemplatePasswordReset
)

var previewSubjects = map[string]string{
	signupCodeTemplate:    "注册验证邮件",
	passwordResetTemplate: "密码重置",
}

type controller struct {
	mailer   Mailer
	settings Settings
}

func newController(mailer Mailer, settings Settings) *controller {
	return &controller{mailer: mailer, settings: settings}
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.POST("/admin/mail/messages", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.create)
	r.GET("/admin/mail/status", guard.RequirePermission(rbac.PermissionRegisterView), c.status)
}

func (c *controller) create(ctx *gin.Context) {
	var req createMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	addr, err := mail.ParseAddress(req.To)
	if err != nil || addr.Address == "" {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "to"))
		return
	}

	templateName := strings.TrimSpace(req.Template)
	if templateName == "" {
		templateName = exampleTemplate
	}
	if !isSupportedTemplate(templateName) {
		apierror.Write(ctx, apierror.WithParam(apierror.ErrInvalidRequest, "template"))
		return
	}

	data, err := c.previewData(ctx.Request.Context(), addr.Address)
	if err != nil {
		slog.Error("mail: read preview data", "template", templateName, "err", err)
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}
	subject, body, err := Compose(ctx.Request.Context(), c.settings, templateName, data)
	if err != nil {
		slog.Error("mail: render message", "template", templateName, "err", err)
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}

	msg := Message{To: addr.Address, Subject: subject, Body: body}
	if err := c.mailer.SendSync(ctx.Request.Context(), msg); err != nil {
		apierror.Write(ctx, apierror.ErrMailDeliveryFailed)
		return
	}

	ctx.JSON(http.StatusCreated, createMessageResponse{To: msg.To, Subject: msg.Subject, Sent: true})
}

func (c *controller) status(ctx *gin.Context) {
	configured, err := c.mailer.Configured(ctx.Request.Context())
	if err != nil {
		apierror.Write(ctx, err)
		return
	}

	siteURLConfigured := false
	if c.settings != nil {
		values, err := c.settings.GetGroup(ctx.Request.Context(), "site")
		if err != nil {
			apierror.Write(ctx, err)
			return
		}
		raw, _ := values["url"].(string)
		if _, err := SiteURL(raw); err == nil {
			siteURLConfigured = true
		}
	}

	ctx.JSON(http.StatusOK, statusResponse{SMTPConfigured: configured, SiteURLConfigured: siteURLConfigured})
}

func Compose(ctx context.Context, settings Settings, templateName string, data map[string]string) (string, Body, error) {
	body, err := Render(templateName, data)
	if err != nil {
		return "", Body{}, err
	}
	if templateName == exampleTemplate {
		return selfCheckSubject, body, nil
	}

	subject := previewSubjects[templateName]
	if settings == nil {
		return subject, body, nil
	}

	values, err := settings.GetGroup(ctx, settingsGroup)
	if err != nil {
		return "", Body{}, fmt.Errorf("mail: read template settings: %w", err)
	}

	if custom, ok := values[templateName+"_subject"].(string); ok && strings.TrimSpace(custom) != "" {
		subject = custom
	}
	if custom, ok := values[templateName+"_body"].(string); ok && strings.TrimSpace(custom) != "" {
		html, err := renderHTML(custom, data)
		if err != nil {
			return "", Body{}, err
		}
		body.HTML = html
	}

	return subject, body, nil
}

func (c *controller) previewData(ctx context.Context, to string) (map[string]string, error) {
	var siteTitle string
	var siteURL string

	if c.settings != nil {
		site, err := c.settings.GetGroup(ctx, "site")
		if err != nil {
			return nil, fmt.Errorf("mail: read site settings: %w", err)
		}
		if value, ok := site["title"].(string); ok {
			siteTitle = value
		}
		if value, ok := site["url"].(string); ok {
			siteURL = value
		}
	}

	username := to
	if at := strings.Index(to, "@"); at > 0 {
		username = to[:at]
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	return map[string]string{
		"Site":           siteTitle,
		"Time":           now,
		"code":           "123456",
		"expiresMinutes": "30",
		"siteTitle":      siteTitle,
		"siteUrl":        siteURL,
		"username":       username,
		"sentAt":         now,
		"verifyUrl":      "https://example.com/register/verify?token=preview-token",
		"resetUrl":       "https://example.com/reset-password/confirm?token=preview-token",
	}, nil
}

func isSupportedTemplate(name string) bool {
	switch name {
	case exampleTemplate, signupCodeTemplate, passwordResetTemplate:
		return true
	default:
		return false
	}
}
