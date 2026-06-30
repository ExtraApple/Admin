package handler

import (
	"net/http"

	"admin/dto"
	"admin/service"

	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct{}

// ListAuditLogs 查询所有审计日志，支持分页和通用筛选条件。
func (h *AuditLogHandler) ListAuditLogs(c *gin.Context) {
	h.list(c)
}

// ListLoginLogs 查询登录分类的审计日志。
func (h *AuditLogHandler) ListLoginLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryLogin)
}

// ListOperationLogs 查询增删改操作日志，并包含权限变更类操作。
func (h *AuditLogHandler) ListOperationLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryOperation, service.AuditCategoryPermission)
}

// ListPermissionLogs 查询角色、权限、菜单相关的变更日志。
func (h *AuditLogHandler) ListPermissionLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryPermission)
}

// ListDataAccessLogs 查询 GET 请求产生的数据访问日志。
func (h *AuditLogHandler) ListDataAccessLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryDataAccess)
}

// list 执行审计日志的通用查询并组装统一响应。
func (h *AuditLogHandler) list(c *gin.Context) {
	req := bindAuditLogListReq(c)
	list, total, err := service.ListAuditLogs(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "查询日志失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.AuditLogListResp{
		List:  list,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}})
}

// listByCategories 按指定分类集合查询审计日志。
func (h *AuditLogHandler) listByCategories(c *gin.Context, categories ...string) {
	req := bindAuditLogListReq(c)
	list, total, err := service.ListAuditLogsByCategories(req, categories...)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "查询日志失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.AuditLogListResp{
		List:  list,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}})
}

// bindAuditLogListReq 绑定日志查询参数，并补充分页默认值。
func bindAuditLogListReq(c *gin.Context) dto.AuditLogListReq {
	var req dto.AuditLogListReq
	_ = c.ShouldBindQuery(&req)
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 10
	}
	return req
}
