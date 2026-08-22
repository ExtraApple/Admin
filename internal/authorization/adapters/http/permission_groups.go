package httpadapter

import (
	"github.com/gin-gonic/gin"
)

type createGroupRequest struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
	Sort int    `json:"sort"`
}

type updateGroupRequest struct {
	Name string `json:"name" binding:"max=50"`
	Sort *int   `json:"sort"`
}

type groupResponse struct {
	Code int       `json:"code"`
	Msg  string    `json:"msg"`
	Data groupInfo `json:"data"`
}

type groupListResponse struct {
	Code int       `json:"code"`
	Data groupList `json:"data"`
}

type groupList struct {
	List  []groupInfo `json:"list"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

type groupInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

func (handler *Handler) listGroups(c *gin.Context) {
	page, size := queryPage(c)
	result, err := handler.service.ListPermissionGroups(c.Request.Context(), page, size)
	if err != nil {
		badRequest(c, err)
		return
	}
	list := make([]groupInfo, len(result.List))
	for i := range result.List {
		list[i] = groupInfo{ID: result.List[i].ID, Name: result.List[i].Name, Sort: result.List[i].Sort}
	}
	success(c, groupList{List: list, Total: result.Total, Page: result.Page, Size: result.Size})
}

func (handler *Handler) createGroup(c *gin.Context) {
	var req createGroupRequest
	if !bindJSON(c, &req) {
		return
	}
	group, err := handler.service.CreatePermissionGroup(c.Request.Context(), req.Name, req.Sort)
	if err != nil {
		badRequest(c, err)
		return
	}
	success(c, groupInfo{ID: group.ID, Name: group.Name, Sort: group.Sort})
}

func (handler *Handler) updateGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req updateGroupRequest
	if !bindJSON(c, &req) {
		return
	}
	group, err := handler.service.UpdatePermissionGroup(c.Request.Context(), id, req.Name, req.Sort)
	if err != nil {
		badRequest(c, err)
		return
	}
	success(c, groupInfo{ID: group.ID, Name: group.Name, Sort: group.Sort})
}

func (handler *Handler) deleteGroup(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeletePermissionGroup(c.Request.Context(), id); err != nil {
		badRequest(c, err)
		return
	}
	success(c, nil)
}
