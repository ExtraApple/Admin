package service

import (
	"errors"
	"io"
	"path/filepath"

	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/utils"
)

const fileBucket = "files"

// UploadFile 上传文件，存入 MinIO 并在 files 表记录元数据
func UploadFile(uploaderID uint, fileName, contentType string, size int64, reader io.Reader) (*dto.FileInfo, error) {
	if size > 50*1024*1024 {
		return nil, errors.New("文件大小不能超过 50MB")
	}

	ext := filepath.Ext(fileName)
	objName, err := utils.UploadStream(fileBucket, "", ext, contentType, reader, size)
	if err != nil {
		return nil, errors.New("文件上传失败: " + err.Error())
	}

	file := model.File{
		Name:        fileName,
		Bucket:      fileBucket,
		ObjectName:  objName,
		ContentType: contentType,
		Size:        size,
		UploaderID:  uploaderID,
	}
	if err := global.DB.Create(&file).Error; err != nil {
		utils.RemoveFile(fileBucket, objName)
		return nil, errors.New("文件记录创建失败")
	}

	return toFileInfo(&file), nil
}

// GetFile 获取文件详情 + 预签名下载链接
func GetFile(fileID uint) (*dto.FileInfo, string, error) {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", errors.New("文件不存在")
		}
		return nil, "", errors.New("查询文件失败")
	}

	url, err := utils.DownloadURL(file.Bucket, file.ObjectName, 300)
	if err != nil {
		return nil, "", errors.New("生成下载链接失败")
	}

	return toFileInfo(&file), url, nil
}

// ListFiles 获取文件列表（分页，支持按前缀筛选）
func ListFiles(page, pageSize int, prefix string) ([]dto.FileInfo, int64, error) {
	var files []model.File
	var total int64

	query := global.DB.Model(&model.File{})
	if prefix != "" {
		query = query.Where("object_name LIKE ?", prefix+"%")
	}
	query.Count(&total)
	if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&files).Error; err != nil {
		return nil, 0, errors.New("查询文件列表失败")
	}

	list := make([]dto.FileInfo, len(files))
	for i, f := range files {
		list[i] = *toFileInfo(&f)
	}
	return list, total, nil
}

// UpdateFile 修改文件元信息（仅文件名）
func UpdateFile(fileID uint, req dto.UpdateFileReq) (*dto.FileInfo, error) {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("文件不存在")
		}
		return nil, errors.New("查询文件失败")
	}

	if req.Name != "" {
		if err := global.DB.Model(&file).Update("name", req.Name).Error; err != nil {
			return nil, errors.New("修改失败")
		}
		file.Name = req.Name
	}
	return toFileInfo(&file), nil
}

// DeleteFile 删除文件（MinIO + DB 双删）
func DeleteFile(fileID uint) error {
	var file model.File
	if err := global.DB.First(&file, fileID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("文件不存在")
		}
		return errors.New("查询文件失败")
	}

	if err := utils.RemoveFile(file.Bucket, file.ObjectName); err != nil {
		return errors.New("删除文件失败: " + err.Error())
	}
	return global.DB.Unscoped().Delete(&file).Error
}

// BrowseFiles 按目录浏览 MinIO 中的文件
func BrowseFiles(prefix string) ([]utils.FileInfo, error) {
	return utils.ListFiles(fileBucket, prefix)
}

// toFileInfo 将文件模型转换为接口返回结构，并格式化创建时间。
func toFileInfo(f *model.File) *dto.FileInfo {
	return &dto.FileInfo{
		ID:          f.ID,
		Name:        f.Name,
		Bucket:      f.Bucket,
		ObjectName:  f.ObjectName,
		ContentType: f.ContentType,
		Size:        f.Size,
		UploaderID:  f.UploaderID,
		CreatedAt:   f.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
