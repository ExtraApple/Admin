package service

import (
	"time"

	"admin/dto"
	"admin/global"
	"admin/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
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

// queryAuditLogs 执行审计日志的通用分页查询，并转换为接口响应结构。
func queryAuditLogs(query *gorm.DB, req dto.AuditLogListReq) ([]dto.AuditLogInfo, int64, error) {
	req = normalizeAuditLogPage(req)
	query = applyAuditLogFilters(query, req)

	var logs []model.AuditLog
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("created_at desc").
		Limit(req.Size).
		Offset((req.Page - 1) * req.Size).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return toAuditLogInfoList(logs), total, nil
}

// normalizeAuditLogPage 为审计日志查询填充分页默认值。
func normalizeAuditLogPage(req dto.AuditLogListReq) dto.AuditLogListReq {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 10
	}
	return req
}

// applyAuditLogFilters 将基础筛选和时间筛选依次应用到 GORM 查询上。
func applyAuditLogFilters(query *gorm.DB, req dto.AuditLogListReq) *gorm.DB {
	query = applyAuditFieldFilters(query, req)
	query = applyAuditTimeRange(query, req)
	return query
}

// applyAuditFieldFilters 应用用户、方法、路径、状态和分类等普通筛选条件。
func applyAuditFieldFilters(query *gorm.DB, req dto.AuditLogListReq) *gorm.DB {
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.Method != "" {
		query = query.Where("method = ?", req.Method)
	}
	if req.Path != "" {
		query = query.Where("path LIKE ?", "%"+req.Path+"%")
	}
	if req.Status > 0 {
		query = query.Where("status = ?", req.Status)
	}
	if req.Category != "" {
		query = query.Where("category = ?", req.Category)
	}
	return query
}

// applyAuditTimeRange 根据开始和结束时间限制审计日志范围。
func applyAuditTimeRange(query *gorm.DB, req dto.AuditLogListReq) *gorm.DB {
	if req.StartTime != "" {
		if start, err := time.Parse("2006-01-02 15:04:05", req.StartTime); err == nil {
			query = query.Where("created_at >= ?", start)
		}
	}
	if req.EndTime != "" {
		if end, err := time.Parse("2006-01-02 15:04:05", req.EndTime); err == nil {
			query = query.Where("created_at <= ?", end)
		}
	}
	return query
}

// toAuditLogInfoList 将审计日志模型列表转换为响应 DTO 列表。
func toAuditLogInfoList(logs []model.AuditLog) []dto.AuditLogInfo {
	list := make([]dto.AuditLogInfo, len(logs))
	for i, item := range logs {
		list[i] = toAuditLogInfo(item)
	}
	return list
}

// toAuditLogInfo 将单条审计日志模型格式化为接口响应结构。
func toAuditLogInfo(log model.AuditLog) dto.AuditLogInfo {
	return dto.AuditLogInfo{
		ID:        log.ID,
		UserID:    log.UserID,
		Username:  log.Username,
		Method:    log.Method,
		Path:      log.Path,
		Query:     log.Query,
		Body:      log.Body,
		Metadata:  log.Metadata,
		Status:    log.Status,
		Duration:  log.Duration,
		ClientIP:  log.ClientIP,
		UserAgent: log.UserAgent,
		Category:  log.Category,
		CreatedAt: log.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
