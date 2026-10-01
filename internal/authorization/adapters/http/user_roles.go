package httpadapter

import (
 "errors"
 "strings"

 "admin/internal/authorization/application"
 "admin/internal/platform/httpresponse"
 "github.com/gin-gonic/gin"
 "github.com/go-playground/validator/v10"
)

type updateUserRolesRequest struct {
 RoleIDs []uint `json:"role_ids" binding:"required,dive,gt=0"`
 ExpectedAccessVersion int `json:"expected_access_version" binding:"required,gt=0"`
}
type accessVersionData struct{ AccessVersion int `json:"access_version"` }
type accessVersionResponse struct{Data accessVersionData `json:"data"`}

func (handler *Handler) setUserRoles(c *gin.Context){
 targetID,ok:=pathID(c);if !ok{return}
 var request updateUserRolesRequest
 if err:=c.ShouldBindJSON(&request);err!=nil{
  var failures validator.ValidationErrors
  if !errors.As(err,&failures){httpresponse.WriteError(c,httpresponse.RequestInvalidDefinition(),err,nil);return}
  roleInvalid,versionInvalid:=false,false
  for _,failure:=range failures{if strings.HasPrefix(failure.StructField(),"RoleIDs"){roleInvalid=true};if failure.StructField()=="ExpectedAccessVersion"{versionInvalid=true}}
  definition:=authzValidation();fields:=make([]httpresponse.FieldError,0,2)
  for index,invalid:=range []bool{roleInvalid,versionInvalid}{if invalid{field:=definition.Fields[index];fields=append(fields,httpresponse.FieldError{Field:field.Field,ErrorCode:field.Code,Message:field.Message})}}
  httpresponse.WriteError(c,definition,err,httpresponse.ValidationErrorData{Fields:fields});return
 }
 version,err:=handler.service.SetUserRoles(c.Request.Context(),c.GetUint("userID"),targetID,application.UpdateUserRolesRequest{RoleIDs:request.RoleIDs,ExpectedAccessVersion:request.ExpectedAccessVersion})
 if err!=nil{badRequest(c,err);return}
 success(c,accessVersionData{AccessVersion:version})
}
