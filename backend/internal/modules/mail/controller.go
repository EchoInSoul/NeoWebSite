package mail

import (
	"net/http"
	"net/mail"
	"time"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

const selfCheckSubject = "NeoBlog 邮件配置自检"

type controller struct {
	mailer Mailer
}

func newController(mailer Mailer) *controller {
	return &controller{mailer: mailer}
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

	body, err := Render("example", map[string]string{
		"Site": "NeoBlog",
		"Time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}

	msg := Message{To: addr.Address, Subject: selfCheckSubject, Body: body}
	if err := c.mailer.SendSync(ctx.Request.Context(), msg); err != nil {
		apierror.Write(ctx, apierror.ErrMailDeliveryFailed)
		return
	}

	ctx.JSON(http.StatusCreated, createMessageResponse{To: msg.To, Subject: msg.Subject, Sent: true})
}
