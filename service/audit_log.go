package service

import (
	"admin/dto"
	"admin/global"
	"admin/model"

	"go.uber.org/zap"
)

const (
	AuditCategoryAPI        = "api"
	AuditCategoryLogin      = "login"
	AuditCategoryOperation  = "operation"
	AuditCategoryPermission = "permission"
	AuditCategoryDataAccess = "data_access"
)

// 创建审计日志
func CreateAuditLog(log *model.AuditLog) {
	if log.UserID > 0 && log.Username == "" {
		var user model.User
		if err := global.DB.Select("username").First(&user, log.UserID).Error; err == nil {
			log.Username = user.Username
		}
	}

	if err := global.DB.Create(log).Error; err != nil {
		global.Logger.Error("create audit log failed", zap.Error(err))
	}
}

// 列审计日志
func ListAuditLogs(req dto.AuditLogListReq) ([]dto.AuditLogInfo, int64, error) {
	query := global.DB.Model(&model.AuditLog{})
	return queryAuditLogs(query, req)
}

// 分类类出审计日志
func ListAuditLogsByCategories(req dto.AuditLogListReq, categories ...string) ([]dto.AuditLogInfo, int64, error) {
	if len(categories) == 1 {
		req.Category = categories[0]
		return ListAuditLogs(req)
	}

	query := global.DB.Model(&model.AuditLog{}).Where("category IN ?", categories)
	return queryAuditLogs(query, req)
}
