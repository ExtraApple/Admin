package utils

import (
	"context"
	"io"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"admin/global"
)

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
