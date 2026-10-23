package mail

import (
	"context"
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
	selfCheckSubject      = "NeoBlog 邮件配置自检"
	exampleTemplate       = "example"
	signupCodeTemplate    = "signup_code"
	passwordResetTemplate = "password_reset"
)

var previewSubjects = map[string]string{
	signupCodeTemplate:    "NeoBlog 注册验证码",
	passwordResetTemplate: "NeoBlog 密码重置验证码",
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

	subject, body, err := c.renderMessage(ctx.Request.Context(), templateName, addr.Address)
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

func (c *controller) renderMessage(ctx context.Context, templateName, to string) (string, Body, error) {
	data := c.previewData(ctx, to)

	body, err := Render(templateName, data)
	if err != nil {
		return "", Body{}, err
	}
	if templateName == exampleTemplate {
		return selfCheckSubject, body, nil
	}

	subject := previewSubjects[templateName]
	if c.settings == nil {
		return subject, body, nil
	}

	values, err := c.settings.GetGroup(ctx, "mail")
	if err != nil {
		slog.Warn("mail: read template settings", "err", err)
		return subject, body, nil
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

func (c *controller) previewData(ctx context.Context, to string) map[string]string {
	siteTitle := "NeoBlog"
	siteURL := "https://example.com"

	if c.settings != nil {
		site, err := c.settings.GetGroup(ctx, "site")
		if err != nil {
			slog.Warn("mail: read site settings", "err", err)
		} else {
			if value, ok := site["title"].(string); ok && strings.TrimSpace(value) != "" {
				siteTitle = value
			}
			if value, ok := site["url"].(string); ok && strings.TrimSpace(value) != "" {
				siteURL = value
			}
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
		"expiresMinutes": "10",
		"siteTitle":      siteTitle,
		"siteUrl":        siteURL,
		"username":       username,
		"sentAt":         now,
	}
}

func isSupportedTemplate(name string) bool {
	switch name {
	case exampleTemplate, signupCodeTemplate, passwordResetTemplate:
		return true
	default:
		return false
	}
}
