package captcha

import (
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/captcha/turnstile"
	"website/backend/internal/modules/settings"
)

type Module struct {
	Service *Service

	builtinController   *builtin.Controller
	turnstileController *turnstile.Controller
}

func New(db *database.Database, settingsService *settings.Service, limiter *ratelimit.Limiter) (*Module, error) {
	builtinService, err := builtin.New(db, settingsService, limiter)
	if err != nil {
		return nil, err
	}

	turnstileService := turnstile.NewServiceWithPolicies(
		settingsService,
		turnstile.New(turnstile.SiteverifyURL, turnstile.Timeout),
		limiter,
		turnstile.DefaultPolicies(),
	)
	service := newService(settingsService, turnstileService, builtinService)

	return &Module{
		Service:             service,
		builtinController:   builtin.NewController(builtinService),
		turnstileController: turnstile.NewController(turnstileService),
	}, nil
}

func (m *Module) Register(r *gin.RouterGroup) {
	m.builtinController.Register(r)
}

func (m *Module) RegisterProtected(r *gin.RouterGroup) {
	m.turnstileController.RegisterProtected(r)
}
