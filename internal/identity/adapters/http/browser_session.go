package httpadapter

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"reflect"
	"time"

	"admin/internal/identity"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

const refreshCookieName = "admin_refresh"

type BrowserSessionConfig struct {
	Origin string
	Secure bool
}

func (handler *routeHandler) checkBrowser(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if handler.originError != nil || len(c.Request.Header.Values("Origin")) != 1 || c.GetHeader("Origin") != handler.browser.Origin || c.GetHeader("Origin") == "" || len(c.Request.Header.Values("X-Admin-Request")) != 1 || c.GetHeader("X-Admin-Request") != "1" || err != nil || contentType != "application/json" || handler.crossOrigin.Check(c.Request) != nil {
		httpresponse.WriteError(c, authnCSRFInvalid(), nil, nil)
		return false
	}
	return true
}

func bindEmptySessionControl(c *gin.Context) bool {
	var request identity.SessionControlRequest
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&request); err != nil {
		identityRequestError(c, err)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		identityRequestError(c, nil)
		return false
	}
	return true
}

func (handler *routeHandler) setRefreshCookie(c *gin.Context, token string, remove bool) {
	cookie := &http.Cookie{Name: refreshCookieName, Value: token, Path: "/api", HttpOnly: true, Secure: handler.browser.Secure, SameSite: http.SameSiteStrictMode}
	if remove {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	http.SetCookie(c.Writer, cookie)
}

func browserSessionRoute(name, path string, handler gin.HandlerFunc, request, response any, refresh bool) routecatalog.Descriptor {
	requestType := reflect.TypeOf(request)
	descriptor := routecatalog.Descriptor{Method: http.MethodPost, Path: path, Access: routecatalog.Public, Handler: handler, Name: name, Group: "identity", DefaultAuditCategory: "identity", OpenAPI: routecatalog.Operation{
		Summary: name, Request: routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: requestType, Required: true},
		Cookie:    &routecatalog.CookieSecurity{Name: refreshCookieName, Description: "Host-only HttpOnly session refresh cookie; HTTPS Secure; only explicitly configured localhost/127.0.0.1 HTTP development may omit Secure", Path: "/api", SameSite: "Strict", Required: refresh, HTTPOnly: true, Secure: true, HostOnly: true, Session: true},
		Headers:   []routecatalog.Header{{Name: "Origin", Description: "Exact configured browser_session.origin", Required: true}, {Name: "X-Admin-Request", Description: "Browser JSON control request", Required: true, Value: "1"}},
		Responses: map[int]routecatalog.Response{http.StatusOK: routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(response))},
	}}
	definitions := []httpresponse.ErrorDefinition{httpresponse.RequestInvalidDefinition(), authnCSRFInvalid(), identityInternal()}
	if refresh {
		descriptor.OpenAPI.Headers = append(descriptor.OpenAPI.Headers, routecatalog.Header{Name: "X-Refresh-Attempt-ID", Description: "Unpredictable 16–128 character base64url or UUID identifier; reuse the same ID for at most 30 seconds when the result is unknown", Required: true})
		definitions = append(definitions, authnRefreshInvalid(), authnRefreshSuperseded())
	} else if path == "/api/login" {
		definitions = append(definitions, authnCaptchaInvalid(), authnCredentialsInvalid(), authnLoginLocked(), identityAccountDisabled())
	}
	for _, definition := range definitions {
		current, exists := descriptor.OpenAPI.Responses[definition.Status]
		if !exists {
			current = routecatalog.ErrorResponse(definition.Message, definition)
		} else {
			current.Errors = append(current.Errors, definition)
		}
		descriptor.OpenAPI.Responses[definition.Status] = current
	}
	for status, current := range descriptor.OpenAPI.Responses {
		current.Headers = []routecatalog.Header{{Name: "Cache-Control", Description: "Never cache browser authentication control responses", Required: true, Value: "no-store"}}
		if status == http.StatusOK || refresh && status == http.StatusUnauthorized {
			description := "Replace admin_refresh session cookie; Path=/api; HttpOnly; SameSite=Strict; HTTPS Secure; no Domain, Max-Age, or Expires"
			if path == "/api/user/logout" || status == http.StatusUnauthorized {
				description = "Delete admin_refresh with matching Path=/api and Host-only scope; HttpOnly; SameSite=Strict; HTTPS Secure"
			}
			current.Headers = append(current.Headers, routecatalog.Header{Name: "Set-Cookie", Description: description, Required: true})
		}
		descriptor.OpenAPI.Responses[status] = current
	}
	return descriptor
}
