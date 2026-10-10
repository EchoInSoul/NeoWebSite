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

	"website/backend/internal/common/guard"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/health"
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
	handler http.Handler
}

func New(cfg config.Config, db *database.Database) *App {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.Default()

	_ = engine.SetTrustedProxies(nil)

	sessions := newSessionManager(cfg, db)

	api := engine.Group(apiPrefix)

	healthModule := health.New(cfg, db)
	healthModule.Register(api)

	authModule := auth.New(db, sessions)
	authModule.Register(api)
	authModule.RegisterProtected(api.Group("", guard.RequireAuth(sessions)))

	return &App{handler: sessions.LoadAndSave(engine)}
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
	server := &http.Server{
		Addr:              addr,
		Handler:           a.handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	return server.Shutdown(shutdownCtx)
}
