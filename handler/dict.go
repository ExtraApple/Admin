package handler

import (
	"net/http"
	"strconv"

	"admin/dto"
	"admin/service"

	"github.com/gin-gonic/gin"
)

type DictHandler struct{}

// ListDictTypes 分页查询字典类型，支持关键字和状态筛选。
func (h *DictHandler) ListDictTypes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	status := parseOptionalInt(c.Query("status"))

	list, total, err := service.GetDictTypes(page, size, c.Query("keyword"), status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.DictTypeListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// CreateDictType 创建新的字典类型。
func (h *DictHandler) CreateDictType(c *gin.Context) {
	var req dto.CreateDictTypeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	dictType, err := service.CreateDictType(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": dictType})
}

// UpdateDictType 修改字典类型；当编码变化时，服务层会同步更新字典项归属。
func (h *DictHandler) UpdateDictType(c *gin.Context) {
	typeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req dto.UpdateDictTypeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	dictType, err := service.UpdateDictType(uint(typeID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": dictType})
}

// DeleteDictType 删除没有字典项引用的字典类型。
func (h *DictHandler) DeleteDictType(c *gin.Context) {
	typeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteDictType(uint(typeID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// ListDictItems 分页查询字典项，支持类型编码、关键字和状态筛选。
func (h *DictHandler) ListDictItems(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	status := parseOptionalInt(c.Query("status"))

	list, total, err := service.GetDictItems(page, size, c.Query("type_code"), c.Query("keyword"), status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dto.DictItemListResp{
		List: list, Total: total, Page: page, Size: size,
	}})
}

// CreateDictItem 在指定字典类型下创建字典项。
func (h *DictHandler) CreateDictItem(c *gin.Context) {
	var req dto.CreateDictItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	item, err := service.CreateDictItem(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": item})
}

// UpdateDictItem 修改字典项，并在服务层校验类型和值的唯一性。
func (h *DictHandler) UpdateDictItem(c *gin.Context) {
	itemID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var req dto.UpdateDictItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	item, err := service.UpdateDictItem(uint(itemID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": item})
}

// DeleteDictItem 删除指定字典项。
func (h *DictHandler) DeleteDictItem(c *gin.Context) {
	itemID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if err := service.DeleteDictItem(uint(itemID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

// ListEnabledItemsByTypeCode 按类型编码返回启用状态的字典项，供前端下拉框使用。
func (h *DictHandler) ListEnabledItemsByTypeCode(c *gin.Context) {
	items, err := service.GetEnabledDictItemsByTypeCode(c.Param("type_code"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// parseOptionalInt 将可选查询参数转换为 *int，空值或非法值返回 nil。
func parseOptionalInt(value string) *int {
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}
