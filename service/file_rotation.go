package service

import (
	"time"

	"admin/global"
	"admin/initialize"
	"admin/model"
	"admin/utils"

	"go.uber.org/zap"
)

// RotateFiles 轮转过期文件：热存储 → 冷存储
func RotateFiles(conf *initialize.Config) {
	if !conf.FileRotation.Enabled {
		global.Logger.Info("file rotation skipped")
		return
	}

	hot := conf.FileRotation.HotBucket
	cold := conf.FileRotation.ColdBucket
	days := conf.FileRotation.Days
	batch := conf.FileRotation.BatchSize

	if batch <= 0 {
		batch = 100
	}

	// 查询超过天数的文件
	cutoff := time.Now().AddDate(0, 0, -days)
	var files []model.File
	result := global.DB.Where("created_at < ? AND bucket = ?", cutoff, hot).
		Limit(batch).Find(&files)

	if result.Error != nil {
		global.Logger.Error("file rotation query failed", zap.Error(result.Error))
		return
	}

	if len(files) == 0 {
		global.Logger.Info("file rotation no files")
		return
	}

	global.Logger.Info("file rotation started",
		zap.Int("count", len(files)),
		zap.String("hot_bucket", hot),
		zap.String("cold_bucket", cold),
	)

	success := 0
	for _, f := range files {
		// 移动文件
		if err := utils.MoveFile(hot, cold, f.ObjectName); err != nil {
			global.Logger.Error("file rotation move failed",
				zap.String("object_name", f.ObjectName),
				zap.Error(err),
			)
			continue
		}

		// 更新数据库记录
		if err := global.DB.Model(&f).Update("bucket", cold).Error; err != nil {
			global.Logger.Error("file rotation db update failed",
				zap.String("object_name", f.ObjectName),
				zap.Error(err),
			)
			// 尝试回滚：移回热存储
			utils.MoveFile(cold, hot, f.ObjectName)
			continue
		}

		success++
	}

	global.Logger.Info("file rotation finished",
		zap.Int("success", success),
		zap.Int("total", len(files)),
	)
}
