package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/dto"
	"admin/service"
)

type APIHandler struct {
	Engine *gin.Engine
}

func (h *APIHandler) ListAPIs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	status := parseOptionalInt(c.Query("status"))
	needAuth := parseOptionalInt(c.Query("need_auth"))
	needAudit := parseOptionalInt(c.Query("need_audit"))

	list, total, err := service.GetAPIs(
		page,
		size,
		c.Query("keyword"),
		c.Query("group"),
		c.Query("method"),
		status,
		needAuth,
		needAudit,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.APIListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

func (h *APIHandler) CreateAPI(c *gin.Context) {
	var req dto.CreateAPIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	api, err := service.CreateAPI(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": api})
}

func (h *APIHandler) UpdateAPI(c *gin.Context) {
	apiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.UpdateAPIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	api, err := service.UpdateAPI(uint(apiID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": api})
}

func (h *APIHandler) DeleteAPI(c *gin.Context) {
	apiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	if err := service.DeleteAPI(uint(apiID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

func (h *APIHandler) GenerateMenuButton(c *gin.Context) {
	apiID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var req dto.GenerateMenuButtonFromAPIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	menu, err := service.GenerateMenuButtonFromAPI(uint(apiID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "生成成功", "data": menu})
}

func (h *APIHandler) SyncAPIs(c *gin.Context) {
	routes := h.Engine.Routes()
	routeList := make([]dto.SyncAPIItem, len(routes))
	for i, route := range routes {
		routeList[i] = dto.SyncAPIItem{
			Method: route.Method,
			Path:   route.Path,
		}
	}

	created, err := service.SyncAPIs(routeList)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "同步成功", "data": gin.H{
		"created": created,
		"count":   len(created),
	}})
}

func (h *APIHandler) SyncAPIPermissions(c *gin.Context) {
	created, updatedAPI, err := service.SyncAPIPermissions()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "同步成功", "data": gin.H{
		"created":       created,
		"created_count": len(created),
		"updated_api":   updatedAPI,
	}})
}
