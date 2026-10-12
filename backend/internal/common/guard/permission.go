package guard

import (
	"context"
	"errors"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/apierror"
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
			apierror.Abort(c, apierror.ErrUnauthorized)
			return
		}

		identity, err := resolver.IdentityByUserID(c.Request.Context(), userID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				_ = sessions.Destroy(c.Request.Context())
				apierror.Abort(c, apierror.ErrUnauthorized)
				return
			}
			apierror.Abort(c, apierror.ErrInternal)
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
			apierror.Abort(c, apierror.ErrInternal)
			return
		}
		for _, held := range identity.Permissions {
			if held == permission {
				c.Next()
				return
			}
		}
		apierror.Abort(c, apierror.ErrForbidden)
	}
}

func RequirePresetGroup(keys ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	return func(c *gin.Context) {
		identity, ok := IdentityFrom(c)
		if !ok {
			apierror.Abort(c, apierror.ErrInternal)
			return
		}
		if _, ok := allowed[identity.GroupKey]; !ok {
			apierror.Abort(c, apierror.ErrForbidden)
			return
		}
		c.Next()
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
