package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"website/backend/internal/config"
)

func newTestRouter(t *testing.T, p pinger) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	New(config.Config{ServiceName: "s"}, p).Register(engine.Group("/api/v1"))
	return engine
}

func TestHealthyReturns200WhenDatabaseOK(t *testing.T) {
	w := httptest.NewRecorder()
	newTestRouter(t, fakePinger{}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthy", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"database":"ok"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestHealthyReturns503WhenDatabaseDown(t *testing.T) {
	w := httptest.NewRecorder()
	newTestRouter(t, fakePinger{err: errors.New("boom")}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/healthy", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"database":"error"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"degraded"`) {
		t.Fatalf("expected degraded status, got %s", w.Body.String())
	}
}
