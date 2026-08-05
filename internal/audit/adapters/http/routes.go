package httpadapter

import (
	"bytes"
	"io"
	nethttp "net/http"
	"reflect"
	"strings"
	"time"

	"admin/internal/audit"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type Recorder interface{ Submit(audit.AuditLog) }
type MiddlewareOptions struct{ Clock audit.Clock }

func NewMiddleware(recorder Recorder, options ...MiddlewareOptions) gin.HandlerFunc {
	var clock audit.Clock
	if len(options) > 0 {
		clock = options[0].Clock
	}
	if clock == nil {
		clock = wallClock{}
	}
	return func(c *gin.Context) {
		start := clock.Now()
		body := readBody(c)
		c.Next()
		if recorder == nil {
			return
		}
		recorder.Submit(audit.AuditLog{UserID: c.GetUint("userID"), Username: c.GetString("username"), Method: c.Request.Method,
			Path: c.Request.URL.Path, Query: c.Request.URL.RawQuery, Body: audit.SanitizeBody(body), Status: c.Writer.Status(),
			Duration: clock.Now().Sub(start).Milliseconds(), ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
			Category: audit.Classify(c.Request.Method, c.Request.URL.Path), Metadata: metadata(c), CreatedAt: start})
	}
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func readBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		return []byte("[multipart omitted]")
	}
	if c.Request.Method == nethttp.MethodGet || c.Request.Method == nethttp.MethodHead {
		return nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(nil))
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

func metadata(c *gin.Context) []byte {
	value, ok := c.Get(audit.UploadAuditMetadataContextKey)
	if !ok {
		return nil
	}
	return audit.UploadMetadata(value)
}

type httpHandler struct{ service *audit.QueryService }
type errorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func Routes(service *audit.QueryService) []routecatalog.Descriptor {
	handler := &httpHandler{service: service}
	return []routecatalog.Descriptor{
		route(nethttp.MethodGet, "/api/admin/audit-logs", "List Audit Logs", "admin.audit-logs.get", handler.list, audit.AuditCategoryAPI),
		route(nethttp.MethodGet, "/api/admin/login-logs", "List Login Logs", "admin.login-logs.get", handler.login, audit.AuditCategoryLogin),
		route(nethttp.MethodGet, "/api/admin/operation-logs", "List Operation Logs", "admin.operation-logs.get", handler.operation, audit.AuditCategoryOperation),
		route(nethttp.MethodGet, "/api/admin/permission-logs", "List Permission Logs", "admin.permission-logs.get", handler.permission, audit.AuditCategoryPermission),
		route(nethttp.MethodGet, "/api/admin/data-access-logs", "List Data Access Logs", "admin.data-access-logs.get", handler.dataAccess, audit.AuditCategoryDataAccess),
	}
}

func route(method, path, name, permission string, handler gin.HandlerFunc, category string) routecatalog.Descriptor {
	return routecatalog.Descriptor{Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler, Name: name, Group: "audit", DefaultPermissionCode: permission, DefaultAuditCategory: category,
		OpenAPI: routecatalog.Operation{Summary: name, Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{
			nethttp.StatusOK:         {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(audit.AuditLogListResponse{})},
			nethttp.StatusBadRequest: {Description: "bad request", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(errorResponse{})},
		}}}
}

func (handler *httpHandler) list(c *gin.Context) { handler.respond(c, nil) }
func (handler *httpHandler) login(c *gin.Context) {
	handler.respond(c, []string{audit.AuditCategoryLogin})
}
func (handler *httpHandler) operation(c *gin.Context) {
	handler.respond(c, []string{audit.AuditCategoryOperation, audit.AuditCategoryPermission})
}
func (handler *httpHandler) permission(c *gin.Context) {
	handler.respond(c, []string{audit.AuditCategoryPermission})
}
func (handler *httpHandler) dataAccess(c *gin.Context) {
	handler.respond(c, []string{audit.AuditCategoryDataAccess})
}

func (handler *httpHandler) respond(c *gin.Context, categories []string) {
	request := audit.AuditLogListRequest{}
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(nethttp.StatusBadRequest, errorResponse{Code: 400, Msg: "查询日志失败: " + err.Error()})
		return
	}
	request = audit.NormalizePage(request)
	var list []audit.AuditLogInfo
	var total int64
	var err error
	if len(categories) == 0 {
		list, total, err = handler.service.List(c.Request.Context(), request)
	} else {
		list, total, err = handler.service.ListByCategories(c.Request.Context(), request, categories...)
	}
	if err != nil {
		c.JSON(nethttp.StatusBadRequest, errorResponse{Code: 400, Msg: "查询日志失败: " + err.Error()})
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"code": 200, "data": audit.AuditLogListResponse{List: list, Total: total, Page: request.Page, Size: request.Size}})
}
