package guard

import (
	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
)

const (
	SessionUserIDKey      = "auth.user_id"
	SessionAuthVersionKey = "auth.version"
)

func RequireAuth(sm *scs.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := sm.GetString(c.Request.Context(), SessionUserIDKey)
		if userID == "" {
			apierror.Abort(c, apierror.ErrUnauthorized)
			return
		}
		c.Set(SessionUserIDKey, userID)
		c.Next()
	}
}
