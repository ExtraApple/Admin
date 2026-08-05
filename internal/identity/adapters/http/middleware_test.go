package httpadapter_test

import (
	"net/http/httptest"
	"testing"

	httpadapter "admin/internal/identity/adapters/http"

	"github.com/gin-gonic/gin"
)

func TestAuthenticationMiddlewareRejectsMissingAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/protected", httpadapter.AuthMiddleware(nil), func(c *gin.Context) { c.Status(204) })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/protected", nil))
	if response.Code != 401 {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}
