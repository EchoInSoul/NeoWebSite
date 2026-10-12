package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/access"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/health"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/settings"
)

const (
	apiPrefix         = "/api/v1"
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type App struct {
	handler  http.Handler
	server   *http.Server
	mailer   mail.Mailer
	settings *settings.Service
}

func New(cfg config.Config, db *database.Database) (*App, error) {
	settingsService, err := settings.New(db)
	if err != nil {
		return nil, err
	}

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Logger(), recoveryHandler())

	_ = engine.SetTrustedProxies(nil)

	sessions := newSessionManager(cfg, db)

	api := engine.Group(apiPrefix)

	healthModule := health.New(cfg, db)
	healthModule.Register(api)

	mailModule := mail.New(cfg)

	authModule := auth.New(db, sessions)
	authModule.Register(api)

	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sessions), guard.LoadIdentity(accessModule.Service, sessions))
	authModule.RegisterProtected(protected)
	accessModule.RegisterProtected(protected)
	mailModule.RegisterProtected(protected)

	engine.NoRoute(func(c *gin.Context) {
		apierror.Write(c, apierror.ErrNotFound)
	})

	handler := sessions.LoadAndSave(engine)

	return &App{
		handler: handler,
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		mailer:   mailModule.Mailer,
		settings: settingsService,
	}, nil
}

func recoveryHandler() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		apierror.Abort(c, apierror.ErrInternal)
	})
}

func newSessionManager(cfg config.Config, db *database.Database) *scs.SessionManager {
	sm := scs.New()
	sm.Lifetime = cfg.SessionLifetime
	sm.Store = postgresstore.New(db.SQL())
	sm.Cookie.Name = "session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	sm.Cookie.Secure = cfg.Environment == "production"
	return sm
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) Run(addr string) error {
	a.server.Addr = addr

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return a.shutdown(shutdownCtx)
}

func (a *App) shutdown(ctx context.Context) error {
	err := a.server.Shutdown(ctx)
	if closeErr := a.mailer.Close(ctx); err == nil {
		err = closeErr
	}
	return err
}
