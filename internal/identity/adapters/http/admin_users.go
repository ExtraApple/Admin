package httpadapter

import (
 "net/http"
 "reflect"

 "admin/internal/identity/application"
 "admin/internal/platform/httpresponse"
 "admin/internal/routecatalog"
 "github.com/gin-gonic/gin"
)

type AdminUserListQuery struct {
 Page int `form:"page" binding:"omitempty,min=1"`
 Size int `form:"size" binding:"omitempty,min=1"`
 Keyword string `form:"keyword"`
 Status *int `form:"status" binding:"omitempty,oneof=0 1"`
}
type adminUserListResponse struct {Data application.AdminUserPage `json:"data"`}
type adminUserDetailResponse struct {Data application.AdminUser `json:"data"`}

type adminUserHandler struct {users *application.AdminUserService}

func AdminUserRoutes(users *application.AdminUserService)[]routecatalog.Descriptor{
 handler:=&adminUserHandler{users:users}
 routes:=[]routecatalog.Descriptor{
  permissionIdentityRoute(http.MethodGet,"/api/admin/users","List Users","admin.users.get",handler.list,nil,adminUserListResponse{}),
  permissionIdentityRoute(http.MethodGet,"/api/admin/users/:id","Get User","admin.users.id.get",handler.get,nil,adminUserDetailResponse{}),
 }
 routes[0].OpenAPI.QuerySchema=reflect.TypeOf(AdminUserListQuery{})
 for index:=range routes{routes[index].OpenAPI.Responses[http.StatusNotFound]=routecatalog.ErrorResponse("user was not found",identityUserNotFound())}
 return routes
}
func identityUserNotFound()httpresponse.ErrorDefinition{
 return httpresponse.ErrorDefinition{Owner:"identity",Code:"IDENTITY_USER_NOT_FOUND",Status:http.StatusNotFound,Message:"user was not found"}
}
func writeAdminUserError(c *gin.Context,err error){
 if code,_:=application.CodeOf(err);code==application.CodeUserNotFound{httpresponse.WriteError(c,identityUserNotFound(),err,nil);return}
 writeIdentityError(c,err,false)
}
func (handler *adminUserHandler) list(c *gin.Context){
 query:=AdminUserListQuery{Page:1,Size:10}
 if err:=c.ShouldBindQuery(&query);err!=nil{httpresponse.WriteError(c,identityValidationInvalid(),err,nil);return}
 result,err:=handler.users.List(c.Request.Context(),c.GetUint("userID"),query.Page,query.Size,application.UserFilter{Keyword:query.Keyword,Status:query.Status})
 if err!=nil{writeAdminUserError(c,err);return}
 identitySuccess(c,result)
}
func (handler *adminUserHandler) get(c *gin.Context){
 targetID,ok:=parseTargetID(c);if !ok{return}
 result,err:=handler.users.Get(c.Request.Context(),c.GetUint("userID"),targetID)
 if err!=nil{writeAdminUserError(c,err);return}
 identitySuccess(c,result)
}
