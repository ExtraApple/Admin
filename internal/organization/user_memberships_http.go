package organization

import (
 "errors"
 "net/http"
 "strings"

 "admin/internal/platform/httpresponse"
 "admin/internal/routecatalog"
 "github.com/gin-gonic/gin"
 "github.com/go-playground/validator/v10"
)

type updateUserOrganizationsRequest struct {
 OrganizationIDs []uint `json:"organization_ids" binding:"required,dive,gt=0"`
 ExpectedAccessVersion int `json:"expected_access_version" binding:"required,gt=0"`
}
type accessVersionData struct{AccessVersion int `json:"access_version"`}
type accessVersionResponse struct{Data accessVersionData `json:"data"`}
type userMembershipHTTPHandler struct{service *UserMembershipService}
func UserMembershipRoutes(service *UserMembershipService)[]routecatalog.Descriptor{
 handler:=&userMembershipHTTPHandler{service:service}
 descriptor:=organizationRoute(http.MethodPut,"/api/admin/users/:id/organizations","Set User Organizations","admin.users.id.organizations.put",handler.set,updateUserOrganizationsRequest{},accessVersionResponse{})
 descriptor.OpenAPI.Responses[http.StatusForbidden]=routecatalog.ErrorResponse("organization operation is not permitted",orgPermissionDenied())
 return []routecatalog.Descriptor{descriptor}
}
func (handler *userMembershipHTTPHandler) set(c *gin.Context){
 targetID,ok:=pathID(c);if !ok{return}
 var request updateUserOrganizationsRequest
 if err:=c.ShouldBindJSON(&request);err!=nil{
  var failures validator.ValidationErrors
  if !errors.As(err,&failures){httpresponse.WriteError(c,httpresponse.RequestInvalidDefinition(),err,nil);return}
  orgInvalid,versionInvalid:=false,false
  for _,failure:=range failures{if strings.HasPrefix(failure.StructField(),"OrganizationIDs"){orgInvalid=true};if failure.StructField()=="ExpectedAccessVersion"{versionInvalid=true}}
  definition:=orgValidation();fields:=make([]httpresponse.FieldError,0,2)
  for index,invalid:=range []bool{orgInvalid,versionInvalid}{if invalid{field:=definition.Fields[index];fields=append(fields,httpresponse.FieldError{Field:field.Field,ErrorCode:field.Code,Message:field.Message})}}
  httpresponse.WriteError(c,definition,err,httpresponse.ValidationErrorData{Fields:fields});return
 }
 version,err:=handler.service.Set(c.Request.Context(),c.GetUint("userID"),targetID,UpdateUserOrganizationsRequest{OrganizationIDs:request.OrganizationIDs,ExpectedAccessVersion:request.ExpectedAccessVersion})
 if err!=nil{organizationError(c,err);return}
 organizationSuccess(c,accessVersionData{AccessVersion:version})
}
