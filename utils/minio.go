package utils

import (
	"context"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"admin/global"
)

// FileInfo 文件基本信息
type FileInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
}

// UploadFile 上传本地文件到 MinIO，返回 objectName（prefix + uuid + ext）
func UploadFile(bucket, prefix, localPath, contentType string) (string, error) {
	objName := prefix + uuid.New().String() + filepath.Ext(localPath)
	_, err := global.Minio.FPutObject(context.Background(), bucket, objName, localPath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return objName, nil
}

// UploadStream 上传 io.Reader 到 MinIO，返回 objectName（prefix + uuid + ext）
func UploadStream(bucket, prefix, ext, contentType string, reader io.Reader, size int64) (string, error) {
	objName := prefix + uuid.New().String() + ext
	_, err := global.Minio.PutObject(context.Background(), bucket, objName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	return objName, nil
}

// CleanOldFiles 保留前缀目录下最新的 keep 个文件，删除其余旧文件
func CleanOldFiles(bucket, prefix string, keep int) {
	ctx := context.Background()
	objects := global.Minio.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	type item struct {
		key          string
		lastModified int64
	}
	var items []item
	for obj := range objects {
		if obj.Err != nil {
			continue
		}
		items = append(items, item{key: obj.Key, lastModified: obj.LastModified.Unix()})
	}
	if len(items) <= keep {
		return
	}
	// 按时间升序，最老的在前
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].lastModified > items[j].lastModified {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	// 删除最老的 N-keep 个
	for i := 0; i < len(items)-keep; i++ {
		global.Minio.RemoveObject(ctx, bucket, items[i].key, minio.RemoveObjectOptions{})
	}
}

// DownloadURL 生成预签名下载 URL，expireSeconds 秒后过期
func DownloadURL(bucket, objName string, expireSeconds int) (string, error) {
	// 生成预签名下载 URL，过期时间为 expireSeconds 秒
	url, err := global.Minio.PresignedGetObject(context.Background(), bucket, objName, time.Duration(expireSeconds)*time.Second, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

// GetFileInfo 获取文件元信息
func GetFileInfo(bucket, objName string) (*FileInfo, error) {
	info, err := global.Minio.StatObject(context.Background(), bucket, objName, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}
	return &FileInfo{
		Name:         info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

// RemoveFile 删除单个文件
func RemoveFile(bucket, objName string) error {
	return global.Minio.RemoveObject(context.Background(), bucket, objName, minio.RemoveObjectOptions{})
}

// ListFiles 获取指定路径下的文件列表
func ListFiles(bucket, prefix string) ([]FileInfo, error) {
	ctx := context.Background()
	objects := global.Minio.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var list []FileInfo
	for obj := range objects {
		if obj.Err != nil {
			continue
		}
		list = append(list, FileInfo{
			Name:         obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified,
		})
	}
	return list, nil
}

// MoveFile 在 bucket 之间移动文件
func MoveFile(srcBucket, dstBucket, objName string) error {
	ctx := context.Background()

	// 复制到目标 bucket
	_, err := global.Minio.CopyObject(
		ctx,
		minio.CopyDestOptions{Bucket: dstBucket, Object: objName},
		minio.CopySrcOptions{Bucket: srcBucket, Object: objName},
	)
	if err != nil {
		return err
	}

	// 删除源文件
	return global.Minio.RemoveObject(ctx, srcBucket, objName, minio.RemoveObjectOptions{})
}
