package guard

import (
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
)

const (
	SessionUserAgentKey  = "auth.user_agent"
	SessionIPKey         = "auth.ip"
	SessionCreatedAtKey  = "auth.created_at"
	SessionLastSeenAtKey = "auth.last_seen_at"
)

const sessionActivityThrottle = 5 * time.Minute

func TrackSessionActivity(sm *scs.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if sm.GetString(ctx, SessionUserIDKey) != "" {
			seen, err := time.Parse(time.RFC3339, sm.GetString(ctx, SessionLastSeenAtKey))
			if err != nil {
				_ = sm.Destroy(ctx)
				apierror.Abort(c, apierror.ErrUnauthorized)
				return
			}
			if time.Since(seen) >= sessionActivityThrottle {
				sm.Put(ctx, SessionLastSeenAtKey, time.Now().UTC().Format(time.RFC3339))
			}
		}
		c.Next()
	}
}
