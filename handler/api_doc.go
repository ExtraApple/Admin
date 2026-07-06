package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/service"
)

type APIDocHandler struct {
	Engine *gin.Engine
	Config dto.OpenAPIDocConfig
}

// Index 返回 Swagger UI 文档页面。
func (h *APIDocHandler) Index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(apiDocHTML))
}

// OpenAPIJSON 返回当前服务路由生成的 OpenAPI JSON 文档。
func (h *APIDocHandler) OpenAPIJSON(c *gin.Context) {
	routes := h.Engine.Routes()
	routeList := make([]dto.OpenAPIRoute, 0, len(routes))
	for _, route := range routes {
		routeList = append(routeList, dto.OpenAPIRoute{
			Method:  route.Method,
			Path:    route.Path,
			Handler: route.Handler,
		})
	}

	cfg := h.Config
	cfg.ServerURL = resolveAPIDocServerURL(c)
	c.JSON(http.StatusOK, service.BuildOpenAPIDocument(routeList, cfg))
}

// resolveAPIDocServerURL 根据请求头和 Host 推断 OpenAPI server URL。
func resolveAPIDocServerURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	host := c.Request.Host
	if strings.TrimSpace(host) == "" {
		host = "localhost:8080"
	}
	return scheme + "://" + host
}

const apiDocHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Admin API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #f8fafc; }
    #swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/docs/openapi.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      persistAuthorization: true
    });
  </script>
</body>
</html>`
