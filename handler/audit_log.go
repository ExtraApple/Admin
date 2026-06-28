package handler

import (
	"net/http"

	"admin/dto"
	"admin/service"

	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct{}

func (h *AuditLogHandler) ListAuditLogs(c *gin.Context) {
	h.list(c)
}

func (h *AuditLogHandler) ListLoginLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryLogin)
}

func (h *AuditLogHandler) ListOperationLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryOperation, service.AuditCategoryPermission)
}

func (h *AuditLogHandler) ListPermissionLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryPermission)
}

func (h *AuditLogHandler) ListDataAccessLogs(c *gin.Context) {
	h.listByCategories(c, service.AuditCategoryDataAccess)
}

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
