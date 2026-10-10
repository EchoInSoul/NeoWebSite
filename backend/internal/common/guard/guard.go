package guard

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

const SessionUserIDKey = "auth.user_id"

func RequireAuth(sm *scs.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := sm.GetString(c.Request.Context(), SessionUserIDKey)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(SessionUserIDKey, userID)
		c.Next()
	}
}
