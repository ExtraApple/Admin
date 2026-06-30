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

// CreateAuditLog 补全审计日志中的用户名并写入数据库，写入失败只记录运行日志。
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

// ListAuditLogs 按筛选条件分页查询全部审计日志。
func ListAuditLogs(req dto.AuditLogListReq) ([]dto.AuditLogInfo, int64, error) {
	query := global.DB.Model(&model.AuditLog{})
	return queryAuditLogs(query, req)
}

// ListAuditLogsByCategories 按一个或多个分类查询审计日志。
func ListAuditLogsByCategories(req dto.AuditLogListReq, categories ...string) ([]dto.AuditLogInfo, int64, error) {
	if len(categories) == 1 {
		req.Category = categories[0]
		return ListAuditLogs(req)
	}

	query := global.DB.Model(&model.AuditLog{}).Where("category IN ?", categories)
	return queryAuditLogs(query, req)
}
