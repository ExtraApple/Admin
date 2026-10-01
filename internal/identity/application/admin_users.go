package application

import (
 "context"
 "errors"
 "strconv"
 "strings"

 "admin/internal/identity/domain"
)

type UserFilter struct { Keyword string; Status *int }
type UserRoleFact struct { UserID uint; ID uint; Code string; Name string; Status int }
type UserOrganizationFact struct { UserID uint; ID uint; Name string; Manageable bool }
type RoleSummary struct { ID uint `json:"id"`; Code string `json:"code"`; Name string `json:"name"`; Status int `json:"status"` }
type OrganizationSummary struct { ID uint `json:"id"`; Name string `json:"name"` }
type AdminUser struct {
 ID uint `json:"id"`
 Username string `json:"username"`
 Nickname string `json:"nickname"`
 Avatar string `json:"avatar"`
 Email string `json:"email"`
 Status int `json:"status"`
 Roles []RoleSummary `json:"roles"`
 Organizations []OrganizationSummary `json:"organizations"`
 HasUnmanagedOrganizations bool `json:"has_unmanaged_organizations"`
 AccessVersion int `json:"access_version"`
}
type AdminUserPage struct { List []AdminUser `json:"list"`; Total int64 `json:"total"`; Page int `json:"page"`; Size int `json:"size"` }

// AdminAuthorizationReader returns batch authorization facts, never legacy users.role.
type AdminAuthorizationReader interface {
 UserScope(context.Context,uint)(domain.UserScope,error)
 UserRoles(context.Context,[]uint)([]UserRoleFact,error)
 EnsureVersions(context.Context,[]uint)(map[uint]int,error)
}
type AdminOrganizationReader interface { UserOrganizations(context.Context,uint,[]uint)([]UserOrganizationFact,error) }
type AdminUserRepository interface {
 FindByID(context.Context,uint)(domain.User,error)
 List(context.Context,int,int,domain.UserScope,UserFilter)([]domain.User,int64,error)
}

// AdminUserService owns the safe administrative read model separately from self-service writes.
type AdminUserService struct {
 users AdminUserRepository
 authorization AdminAuthorizationReader
 organizations AdminOrganizationReader
 transactions TransactionRunner
}
func NewAdminUserService(users AdminUserRepository,authorization AdminAuthorizationReader,organizations AdminOrganizationReader,transactions TransactionRunner)*AdminUserService{
 return &AdminUserService{users:users,authorization:authorization,organizations:organizations,transactions:transactions}
}
func (service *AdminUserService) List(ctx context.Context,operatorID uint,page,size int,filter UserFilter)(AdminUserPage,error){
 page,size=normalizeUserPage(page,size)
 filter.Keyword=strings.TrimSpace(filter.Keyword)
 if filter.Status!=nil && *filter.Status!=0 && *filter.Status!=1{return AdminUserPage{},NewError(CodeValidationInvalid,nil)}
 scope,err:=service.authorization.UserScope(ctx,operatorID)
 if err!=nil{return AdminUserPage{},NewError(CodeInternalError,err)}
 users,total,err:=service.users.List(ctx,(page-1)*size,size,scope,filter)
 if err!=nil{return AdminUserPage{},NewError(CodeInternalError,err)}
 result,err:=service.project(ctx,operatorID,users)
 if err!=nil{return AdminUserPage{},err}
 return AdminUserPage{List:result,Total:total,Page:page,Size:size},nil
}
func (service *AdminUserService) Get(ctx context.Context,operatorID,targetID uint)(AdminUser,error){
 scope,err:=service.authorization.UserScope(ctx,operatorID)
 if err!=nil{return AdminUser{},NewError(CodeInternalError,err)}
 if !scope.All && !containsUserID(scope.UserIDs,targetID){return AdminUser{},NewError(CodePermissionDenied,nil)}
 user,err:=service.users.FindByID(ctx,targetID)
 if errors.Is(err,ErrUserNotFound){return AdminUser{},NewError(CodeUserNotFound,err)}
 if err!=nil{return AdminUser{},NewError(CodeInternalError,err)}
 result,err:=service.project(ctx,operatorID,[]domain.User{user})
 if err!=nil{return AdminUser{},err}
 return result[0],nil
}
func (service *AdminUserService) project(ctx context.Context,operatorID uint,users []domain.User)([]AdminUser,error){
 result:=make([]AdminUser,len(users))
 if len(users)==0{return result,nil}
 ids:=make([]uint,len(users));indexes:=make(map[uint]int,len(users))
 for index,user:=range users{
  ids[index]=user.ID;indexes[user.ID]=index
  avatar:="/api/avatars/default";if _,trusted:=domain.TrustedAvatarObjectName(user,user.ID);trusted{avatar="/api/avatars/"+strconv.FormatUint(uint64(user.ID),10)}
  result[index]=AdminUser{ID:user.ID,Username:user.Username,Nickname:user.Nickname,Avatar:avatar,Email:user.Email,Status:user.Status,Roles:[]RoleSummary{},Organizations:[]OrganizationSummary{}}
 }
 err:=service.transactions.Run(ctx,func(tx context.Context)error{
  versions,err:=service.authorization.EnsureVersions(tx,ids);if err!=nil{return err}
  roles,err:=service.authorization.UserRoles(tx,ids);if err!=nil{return err}
  organizations,err:=service.organizations.UserOrganizations(tx,operatorID,ids);if err!=nil{return err}
  for id,index:=range indexes {version:=versions[id];if version<1{return NewError(CodeInternalError,nil)};result[index].AccessVersion=version}
  for _,role:=range roles {if index,ok:=indexes[role.UserID];ok{result[index].Roles=append(result[index].Roles,RoleSummary{ID:role.ID,Code:role.Code,Name:role.Name,Status:role.Status})}}
  for _,org:=range organizations {if index,ok:=indexes[org.UserID];ok{if org.Manageable{result[index].Organizations=append(result[index].Organizations,OrganizationSummary{ID:org.ID,Name:org.Name})}else{result[index].HasUnmanagedOrganizations=true}}}
  return nil
 })
 if err!=nil{return nil,NewError(CodeInternalError,err)}
 return result,nil
}
