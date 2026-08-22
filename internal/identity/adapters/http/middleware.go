package httpadapter

import (
	"strings"

	"admin/internal/identity/application"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware(service *application.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.Abort()
			httpresponse.WriteError(c, authnHeaderMissing(), nil, nil)
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" || strings.TrimSpace(parts[1]) == "" {
			c.Abort()
			httpresponse.WriteError(c, authnHeaderInvalid(), nil, nil)
			return
		}
		if service == nil {
			c.Abort()
			httpresponse.WriteError(c, authnTokenInvalid(), nil, nil)
			return
		}
		identity, err := service.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			c.Abort()
			switch code, _ := application.CodeOf(err); code {
			case application.CodeAccountDisabled:
				httpresponse.WriteError(c, identityAccountDisabled(), err, nil)
			case application.CodeInternalError:
				httpresponse.WriteError(c, identityInternal(), err, nil)
			default:
				httpresponse.WriteError(c, authnTokenInvalid(), err, nil)
			}
			return
		}
		c.Set("userID", identity.UserID)
		c.Set("roles", identity.Roles)
		c.Set("permissions", identity.Permissions)
		c.Next()
	}
}
