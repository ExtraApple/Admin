package organization

import (
	"net/http"
	"reflect"
	"strconv"

	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
)

type httpHandler struct {
	service *Service
}

type successResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type errorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type unitResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data UnitInfo `json:"data"`
}

type listEnvelope struct {
	Code int          `json:"code"`
	Data ListResponse `json:"data"`
}

type treeEnvelope struct {
	Code int        `json:"code"`
	Data []TreeNode `json:"data"`
}

type membersEnvelope struct {
	Code int          `json:"code"`
	Data []MemberInfo `json:"data"`
}

func Routes(service *Service) []routecatalog.Descriptor {
	handler := &httpHandler{service: service}
	return []routecatalog.Descriptor{
		organizationRoute(http.MethodGet, "/api/admin/organizations", "List Organization Units", "admin.organizations.get", handler.listUnits, nil, listEnvelope{}),
		organizationRoute(http.MethodGet, "/api/admin/organizations/tree", "Get Organization Tree", "admin.organizations.tree.get", handler.tree, nil, treeEnvelope{}),
		organizationRoute(http.MethodPost, "/api/admin/organizations", "Create Organization Unit", "admin.organizations.post", handler.createUnit, CreateUnitRequest{}, unitResponse{}),
		organizationRoute(http.MethodPut, "/api/admin/organizations/:id", "Update Organization Unit", "admin.organizations.id.put", handler.updateUnit, UpdateUnitRequest{}, unitResponse{}),
		organizationRoute(http.MethodDelete, "/api/admin/organizations/:id", "Delete Organization Unit", "admin.organizations.id.delete", handler.deleteUnit, nil, successResponse{}),
		organizationRoute(http.MethodPost, "/api/admin/organizations/:id/users", "Assign Organization Members", "admin.organizations.id.users.post", handler.assignUsers, AssignUsersRequest{}, successResponse{}),
		organizationRoute(http.MethodGet, "/api/admin/organizations/:id/users", "List Organization Members", "admin.organizations.id.users.get", handler.users, nil, membersEnvelope{}),
	}
}

func organizationRoute(method, path, name, permissionCode string, handler gin.HandlerFunc, requestSchema, responseSchema any) routecatalog.Descriptor {
	request := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if requestSchema != nil {
		request = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(requestSchema), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: routecatalog.PermissionControlled, Handler: handler,
		Name: name, Group: "organization", DefaultPermissionCode: permissionCode, DefaultAuditCategory: "organization",
		OpenAPI: routecatalog.Operation{
			Summary: name,
			Request: request,
			Responses: map[int]routecatalog.Response{
				http.StatusOK:         {Description: "success", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(responseSchema)},
				http.StatusBadRequest: {Description: "bad request", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(errorResponse{})},
			},
		},
	}
}

func (handler *httpHandler) listUnits(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	units, total, err := handler.service.ListUnits(c.Request.Context(), c.GetUint("userID"), page, size, c.Query("keyword"), parseOptionalInt(c.Query("status")))
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": ListResponse{List: units, Total: total, Page: page, Size: size}})
}

func (handler *httpHandler) tree(c *gin.Context) {
	tree, err := handler.service.Tree(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": tree})
}

func (handler *httpHandler) createUnit(c *gin.Context) {
	var request CreateUnitRequest
	if !bindJSON(c, &request) {
		return
	}
	unit, err := handler.service.CreateUnit(c.Request.Context(), c.GetUint("userID"), request)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "创建成功", "data": unit})
}

func (handler *httpHandler) updateUnit(c *gin.Context) {
	unitID, ok := pathID(c)
	if !ok {
		return
	}
	var request UpdateUnitRequest
	if !bindJSON(c, &request) {
		return
	}
	unit, err := handler.service.UpdateUnit(c.Request.Context(), c.GetUint("userID"), unitID, request)
	if err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功", "data": unit})
}

func (handler *httpHandler) deleteUnit(c *gin.Context) {
	unitID, ok := pathID(c)
	if !ok {
		return
	}
	if err := handler.service.DeleteUnit(c.Request.Context(), c.GetUint("userID"), unitID); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

func (handler *httpHandler) assignUsers(c *gin.Context) {
	unitID, ok := pathID(c)
	if !ok {
		return
	}
	var request AssignUsersRequest
	if !bindJSON(c, &request) {
		return
	}
	if err := handler.service.SetUsers(c.Request.Context(), c.GetUint("userID"), unitID, request.UserIDs); err != nil {
		badRequest(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "分配成功"})
}

func (handler *httpHandler) users(c *gin.Context) {
	unitID, ok := pathID(c)
	if !ok {
		return
	}
	users, err := handler.service.Users(c.Request.Context(), c.GetUint("userID"), unitID)
	if err != nil {
		badRequest(c, err)
		return
	}
	response := make([]MemberInfo, len(users))
	for index, user := range users {
		response[index] = MemberInfo{ID: user.ID, Username: user.Username, Nickname: user.Nickname, Avatar: user.Avatar, Email: user.Email, Role: user.Role, Status: user.Status}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": response})
}

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

func pathID(c *gin.Context) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return 0, false
	}
	return uint(parsed), true
}

func bindJSON(c *gin.Context, request any) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return false
	}
	return true
}

func badRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
}
