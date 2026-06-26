package service

import (
	"log"
	"time"

	"admin/global"
	"admin/initialize"
	"admin/model"
	"admin/utils"
)

// RotateFiles 轮转过期文件：热存储 → 冷存储
func RotateFiles(conf *initialize.Config) {
	if !conf.FileRotation.Enabled {
		log.Println("[FileRotation] 未启用，跳过")
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
		log.Printf("[FileRotation] 查询失败: %v", result.Error)
		return
	}

	if len(files) == 0 {
		log.Println("[FileRotation] 无需要轮转的文件")
		return
	}

	log.Printf("[FileRotation] 开始轮转 %d 个文件 (%s → %s)", len(files), hot, cold)

	success := 0
	for _, f := range files {
		// 移动文件
		if err := utils.MoveFile(hot, cold, f.ObjectName); err != nil {
			log.Printf("[FileRotation] 移动失败 [%s]: %v", f.ObjectName, err)
			continue
		}

		// 更新数据库记录
		if err := global.DB.Model(&f).Update("bucket", cold).Error; err != nil {
			log.Printf("[FileRotation] 更新数据库失败 [%s]: %v", f.ObjectName, err)
			// 尝试回滚：移回热存储
			utils.MoveFile(cold, hot, f.ObjectName)
			continue
		}

		success++
	}

	log.Printf("[FileRotation] 完成：成功 %d/%d", success, len(files))
}
