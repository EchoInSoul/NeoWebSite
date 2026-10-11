package guard

import (
	"context"
	"errors"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

const SessionIdentityKey = "auth.identity"

var ErrUserNotFound = errors.New("user not found")

type Identity struct {
	UserID      string
	GroupID     string
	GroupKey    string
	GroupName   string
	Permissions []string
}

type IdentityResolver interface {
	IdentityByUserID(ctx context.Context, userID string) (Identity, error)
}

func LoadIdentity(resolver IdentityResolver, sessions *scs.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := sessions.GetString(c.Request.Context(), SessionUserIDKey)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		identity, err := resolver.IdentityByUserID(c.Request.Context(), userID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				_ = sessions.Destroy(c.Request.Context())
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}

		c.Set(SessionIdentityKey, identity)
		c.Next()
	}
}

func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := IdentityFrom(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		for _, held := range identity.Permissions {
			if held == permission {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	}
}

func IdentityFrom(c *gin.Context) (Identity, bool) {
	value, exists := c.Get(SessionIdentityKey)
	if !exists {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}
