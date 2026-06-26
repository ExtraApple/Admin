package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"admin/request"
	"admin/service"
)

type FileHandler struct{}

// Upload 上传文件
func (h *FileHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请选择文件"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "文件打开失败"})
		return
	}
	defer f.Close()

	uploaderID := c.GetUint("userID")
	info, err := service.UploadFile(uint(uploaderID), file.Filename, file.Header.Get("Content-Type"), file.Size, f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "上传成功", "data": info})
}

// GetFile 获取文件详情 + 下载链接
func (h *FileHandler) GetFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	info, downloadURL, err := service.GetFile(uint(fileID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{
		"file": info, "download_url": downloadURL,
	}})
}

// ListFiles 文件列表
func (h *FileHandler) ListFiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	prefix := c.Query("prefix")

	list, total, err := service.ListFiles(page, size, prefix)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": request.FileListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// UpdateFile 修改文件信息
func (h *FileHandler) UpdateFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req request.UpdateFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	info, err := service.UpdateFile(uint(fileID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": info})
}

// DeleteFile 删除文件
func (h *FileHandler) DeleteFile(c *gin.Context) {
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteFile(uint(fileID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// BrowseFiles 按目录浏览
func (h *FileHandler) BrowseFiles(c *gin.Context) {
	prefix := c.Query("prefix")
	files, err := service.BrowseFiles(prefix)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "浏览文件失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": files})
}
