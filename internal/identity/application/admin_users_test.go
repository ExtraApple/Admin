package application_test

import (
 "context"
 "testing"
 "time"

 identitygorm "admin/internal/identity/adapters/gorm"
 "admin/internal/identity/application"
 "admin/internal/identity/domain"
 platformdatabase "admin/internal/platform/database"
 "admin/testsupport/testutil"
)

type adminAuthorizationFake struct { scope domain.UserScope; roles []application.UserRoleFact; versions map[uint]int }
func (fake adminAuthorizationFake) UserScope(context.Context,uint)(domain.UserScope,error){return fake.scope,nil}
func (fake adminAuthorizationFake) UserRoles(_ context.Context,ids []uint)([]application.UserRoleFact,error){
 result:=[]application.UserRoleFact{}
 for _,role:=range fake.roles { for _,id:=range ids {if role.UserID==id {result=append(result,role);break}} }
 return result,nil
}
func (fake adminAuthorizationFake) EnsureVersions(_ context.Context,ids []uint)(map[uint]int,error){
 result:=make(map[uint]int,len(ids));for _,id:=range ids{result[id]=fake.versions[id];if result[id]==0{result[id]=1}};return result,nil
}
type adminOrganizationsFake []application.UserOrganizationFact
func (fake adminOrganizationsFake) UserOrganizations(_ context.Context,_ uint,ids []uint)([]application.UserOrganizationFact,error){
 result:=[]application.UserOrganizationFact{};for _,org:=range fake{for _,id:=range ids{if org.UserID==id{result=append(result,org);break}}};return result,nil
}

func TestAdminUserReadFiltersBeforePaginationAndHidesUnmanagedOrganizations(t *testing.T){
 db:=testutil.OpenIsolatedSQLite(t)
 if err:=db.AutoMigrate(identitygorm.UserModel{});err!=nil{t.Fatal(err)}
 repository:=identitygorm.NewRepository(db);ctx:=context.Background()
 alice:=domain.User{Username:"alice",Nickname:"运营",Email:"alice@example.test",Password:"private-hash",Role:"legacy-admin",Status:1}
 bob:=domain.User{Username:"bob",Nickname:"运营二",Email:"bob@example.test",Status:1}
 outside:=domain.User{Username:"outside",Nickname:"运营三",Email:"outside@example.test",Status:1}
 disabled:=domain.User{Username:"disabled",Nickname:"运营四",Email:"disabled@example.test",Status:0}
 for _,user:=range []*domain.User{&alice,&bob,&outside,&disabled}{if err:=repository.Create(ctx,user);err!=nil{t.Fatal(err)}}
 off:=0
 if err:=repository.Update(ctx,disabled.ID,application.UserChanges{Status:&off});err!=nil{t.Fatal(err)}
 auth:=adminAuthorizationFake{scope:domain.UserScope{UserIDs:[]uint{alice.ID,bob.ID,disabled.ID}},roles:[]application.UserRoleFact{
 {UserID:alice.ID,ID:11,Code:"reader",Name:"读取",Status:1},{UserID:alice.ID,ID:12,Code:"editor",Name:"编辑",Status:0},
 },versions:map[uint]int{alice.ID:4}}
 orgs:=adminOrganizationsFake{
 {UserID:alice.ID,ID:21,Name:"可管理组织",Manageable:true},
 {UserID:alice.ID,ID:99,Name:"不得泄漏的组织",Manageable:false},
 }
 service:=application.NewAdminUserService(repository,auth,orgs,platformdatabase.NewTransactionRunner(db))
 enabled:=1
 page,err:=service.List(ctx,7,2,1,application.UserFilter{Keyword:"运营",Status:&enabled})
 if err!=nil||page.Total!=2||len(page.List)!=1||page.List[0].ID!=bob.ID||page.Page!=2||page.Size!=1{t.Fatalf("filtered page = %#v, %v",page,err)}
 detail,err:=service.Get(ctx,7,alice.ID)
 if err!=nil{t.Fatal(err)}
 if detail.ID!=alice.ID||detail.Username!="alice"||detail.AccessVersion!=4||len(detail.Roles)!=2||detail.Roles[0].Code!="reader"||detail.Roles[1].Code!="editor"{t.Fatalf("user detail = %#v",detail)}
 if len(detail.Organizations)!=1||detail.Organizations[0].ID!=21||detail.Organizations[0].Name!="可管理组织"||!detail.HasUnmanagedOrganizations{t.Fatalf("organization projection = %#v",detail)}
 if _,err:=service.Get(ctx,7,outside.ID);err==nil{t.Fatal("outside user detail was exposed")}else if code,_:=application.CodeOf(err);code!=application.CodePermissionDenied{t.Fatalf("outside detail code = %q",code)}
 auth.scope=domain.UserScope{All:true}
 service=application.NewAdminUserService(repository,auth,orgs,platformdatabase.NewTransactionRunner(db))
 if _,err:=service.Get(ctx,7,999);err==nil{t.Fatal("missing user detail succeeded")}else if code,_:=application.CodeOf(err);code!=application.CodeUserNotFound{t.Fatalf("missing user code = %q",code)}
 empty,err:=service.List(ctx,7,1,10,application.UserFilter{Keyword:"no-match"})
 if err!=nil||empty.Total!=0||empty.List==nil||len(empty.List)!=0{t.Fatalf("empty filtered page = %#v, %v",empty,err)}
}

func TestAdminUserListDeletedFiltersSoftDeletedOnlyRequiresAllScopeAndMarksProtected(t *testing.T) {
 db := testutil.OpenIsolatedSQLite(t)
 if err := db.AutoMigrate(identitygorm.UserModel{}); err != nil {
  t.Fatal(err)
 }
 repository := identitygorm.NewRepository(db)
 ctx := context.Background()

 active := domain.User{Username: "live", Nickname: "活跃", Email: "live@example.test", Status: 1}
 firstDeleted := domain.User{Username: "del_old", Nickname: "旧删除", Email: "old@example.test", Status: 1}
 secondDeleted := domain.User{Username: "del_admin", Nickname: "受保护删", Email: "admin-del@example.test", Status: 0}
 anonymized := domain.User{Username: "del_gone", Nickname: "已匿名", Email: "gone@example.test", Status: 1}
 for _, user := range []*domain.User{&active, &firstDeleted, &secondDeleted, &anonymized} {
  if err := repository.Create(ctx, user); err != nil {
   t.Fatal(err)
  }
 }
 t1 := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
 t2 := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
 if err := db.Model(&identitygorm.UserModel{}).Where("id = ?", firstDeleted.ID).Update("deleted_at", t1).Error; err != nil {
  t.Fatal(err)
 }
 if err := db.Model(&identitygorm.UserModel{}).Where("id = ?", secondDeleted.ID).Update("deleted_at", t2).Error; err != nil {
  t.Fatal(err)
 }
 if err := repository.Delete(ctx, anonymized.ID); err != nil {
  t.Fatal(err)
 }
 if err := repository.Anonymize(ctx, anonymized.ID, application.AnonymizeUserMutation{
  Username: "deleted_gone_1", Email: "deleted_gone_1@deleted.invalid", Password: "!", AnonymizedAt: t2,
 }); err != nil {
  t.Fatal(err)
 }

 auth := adminAuthorizationFake{
  scope: domain.UserScope{All: false, UserIDs: []uint{firstDeleted.ID}},
  roles: []application.UserRoleFact{
   {UserID: firstDeleted.ID, ID: 11, Code: "reader", Name: "读取", Status: 1},
   {UserID: secondDeleted.ID, ID: 12, Code: "admin", Name: "超级管理员", Status: 0},
  },
  versions: map[uint]int{firstDeleted.ID: 5, secondDeleted.ID: 9},
 }
 orgs := adminOrganizationsFake{
  {UserID: firstDeleted.ID, ID: 21, Name: "研发部", Manageable: true},
 }
 service := application.NewAdminUserService(repository, auth, orgs, platformdatabase.NewTransactionRunner(db))
 if _, err := service.ListDeleted(ctx, 7, 1, 10, ""); err == nil {
  t.Fatal("non-all scope ListDeleted succeeded")
 } else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
  t.Fatalf("non-all scope ListDeleted code = %q, want %q", code, application.CodePermissionDenied)
 }

 auth.scope = domain.UserScope{All: true}
 service = application.NewAdminUserService(repository, auth, orgs, platformdatabase.NewTransactionRunner(db))
 page, err := service.ListDeleted(ctx, 7, 1, 10, "")
 if err != nil {
  t.Fatalf("ListDeleted() error = %v", err)
 }
 if page.Total != 2 || len(page.List) != 2 {
  t.Fatalf("ListDeleted() page = %#v, want 2 recoverable deleted users only", page)
 }
 if page.List[0].ID != secondDeleted.ID || !page.List[0].Protected || page.List[0].Avatar != "/api/avatars/default" || page.List[0].AccessVersion != 9 {
  t.Fatalf("first deleted item (newest) = %#v", page.List[0])
 }
 if page.List[1].ID != firstDeleted.ID || page.List[1].Protected || len(page.List[1].Roles) != 1 || len(page.List[1].Organizations) != 1 || page.List[1].AccessVersion != 5 {
  t.Fatalf("second deleted item = %#v", page.List[1])
 }
 filtered, err := service.ListDeleted(ctx, 7, 1, 10, "old@example")
 if err != nil || filtered.Total != 1 || len(filtered.List) != 1 || filtered.List[0].ID != firstDeleted.ID {
  t.Fatalf("email keyword filtered deleted page = %#v, %v", filtered, err)
 }
}
