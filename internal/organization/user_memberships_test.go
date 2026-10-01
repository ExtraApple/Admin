package organization_test

import (
 "context"
 "reflect"
 "testing"
 "time"

 authgorm "admin/internal/authorization/adapters/gorm"
 "admin/internal/organization"
 platformdatabase "admin/internal/platform/database"
 "admin/testsupport/testutil"
)

type membershipVisibility struct {scope organization.OrganizationScope}
func (fake membershipVisibility) Scope(context.Context,uint)(organization.OrganizationScope,error){return fake.scope,nil}
type membershipTargetPolicy struct{target organization.ManagedUser}
func (fake membershipTargetPolicy) ManagedUser(context.Context,uint,uint)(organization.ManagedUser,error){return fake.target,nil}

func TestUserMembershipsPreserveHiddenAndUnchangedJoinedAtAndUseOneVersion(t *testing.T){
 db:=testutil.OpenIsolatedSQLite(t)
 models:=append(organization.Models(),&authgorm.UserAccessVersion{})
 if err:=db.AutoMigrate(models...);err!=nil{t.Fatal(err)}
 repository:=organization.NewGORMRepository(db);ctx:=context.Background()
 a:=organization.Unit{Name:"A",Code:"a",Status:1};b:=organization.Unit{Name:"B",Code:"b",Status:1};hidden:=organization.Unit{Name:"hidden",Code:"hidden",Status:1}
 for _,unit:=range []*organization.Unit{&a,&b,&hidden}{if err:=repository.CreateUnit(ctx,unit);err!=nil{t.Fatal(err)}}
 joined:=time.Date(2026,8,1,2,3,4,0,time.UTC)
 if err:=db.Create(&[]organization.Membership{{UserID:20,OrganizationID:a.ID,CreatedAt:joined},{UserID:20,OrganizationID:hidden.ID,CreatedAt:joined},{UserID:30,OrganizationID:b.ID,CreatedAt:joined}}).Error;err!=nil{t.Fatal(err)}
 versions:=authgorm.NewAccessVersions(db)
 service:=organization.NewUserMembershipService(repository,membershipVisibility{organization.OrganizationScope{OrganizationIDs:[]uint{a.ID,b.ID}}},membershipTargetPolicy{organization.ManagedUser{Exists:true,Visible:true}},versions,platformdatabase.NewTransactionRunner(db))
 facts,err:=service.UserOrganizations(ctx,7,[]uint{20,30})
 if err!=nil||len(facts)!=3{t.Fatalf("user organizations = %#v, %v",facts,err)}
 hiddenCount:=0
 for _,fact:=range facts{if !fact.Manageable {hiddenCount++;if fact.Name!=""||fact.OrganizationID!=0||fact.UserID!=20{t.Fatalf("hidden identity leaked = %#v",fact)}}}
 if hiddenCount!=1{t.Fatalf("hidden memberships = %d",hiddenCount)}
 version,err:=service.Set(ctx,7,20,organization.UpdateUserOrganizationsRequest{OrganizationIDs:[]uint{a.ID,b.ID,b.ID},ExpectedAccessVersion:1})
 if err!=nil||version!=2{t.Fatalf("first Set = %d, %v",version,err)}
 facts20,err:=repository.MemberOrganizationMemberships(ctx,20)
 if err!=nil||len(facts20)!=3{t.Fatalf("memberships = %#v, %v",facts20,err)}
 var firstAdded time.Time
 for _,fact:=range facts20{if fact.OrganizationID==b.ID{firstAdded=fact.JoinedAt}}
 for _,fact:=range facts20{if fact.OrganizationID==a.ID||fact.OrganizationID==hidden.ID{if !fact.JoinedAt.Equal(joined){t.Fatalf("retained joined_at = %v",fact)}}else if fact.JoinedAt.IsZero()||!fact.JoinedAt.After(joined){t.Fatalf("new joined_at = %v",fact)}}
 if _,err:=service.Set(ctx,7,20,organization.UpdateUserOrganizationsRequest{OrganizationIDs:[]uint{},ExpectedAccessVersion:1});err==nil{t.Fatal("stale write succeeded")}else if code,_:=organization.CodeOf(err);code!=organization.CodeConflict{t.Fatalf("stale code = %q",code)}
 unchanged,_:=repository.MemberOrganizationMemberships(ctx,20);if !reflect.DeepEqual(unchanged,facts20){t.Fatalf("stale write changed memberships = %#v",unchanged)}
 version,err=service.Set(ctx,7,20,organization.UpdateUserOrganizationsRequest{OrganizationIDs:[]uint{},ExpectedAccessVersion:2})
 if err!=nil||version!=3{t.Fatalf("clear = %d, %v",version,err)}
 retained,_:=repository.MemberOrganizationMemberships(ctx,20)
 if len(retained)!=1||retained[0].OrganizationID!=hidden.ID||!retained[0].JoinedAt.Equal(joined){t.Fatalf("hidden membership = %#v",retained)}
 other,_:=repository.MemberOrganizationMemberships(ctx,30);if len(other)!=1||other[0].OrganizationID!=b.ID||!other[0].JoinedAt.Equal(joined){t.Fatalf("other user memberships = %#v",other)}
 version,err=service.Set(ctx,7,20,organization.UpdateUserOrganizationsRequest{OrganizationIDs:[]uint{b.ID},ExpectedAccessVersion:3})
 if err!=nil||version!=4{t.Fatalf("rejoin = %d, %v",version,err)}
 rejoined,err:=repository.MemberOrganizationMemberships(ctx,20)
 if err!=nil||len(rejoined)!=2{t.Fatalf("rejoined memberships = %#v, %v",rejoined,err)}
 for _,fact:=range rejoined{if fact.OrganizationID==b.ID&&!fact.JoinedAt.After(firstAdded){t.Fatalf("rejoined time = %s, original = %s",fact.JoinedAt,firstAdded)}}
}

func TestOrganizationTreeDistinguishesManageableNodesFromPositioningAncestors(t *testing.T) {
 db:=testutil.OpenIsolatedSQLite(t)
 if err:=db.AutoMigrate(organization.Models()...);err!=nil{t.Fatal(err)}
 repository:=organization.NewGORMRepository(db);ctx:=context.Background()
 root:=organization.Unit{Name:"定位祖先",Code:"root",Status:1}
 if err:=repository.CreateUnit(ctx,&root);err!=nil{t.Fatal(err)}
 visible:=organization.Unit{Name:"可管理",Code:"visible",ParentID:root.ID,Status:1}
 other:=organization.Unit{Name:"无关",Code:"other",ParentID:root.ID,Status:1}
 for _,unit:=range []*organization.Unit{&visible,&other}{if err:=repository.CreateUnit(ctx,unit);err!=nil{t.Fatal(err)}}
 service:=organization.NewService(repository,organization.NewHierarchy(repository),membershipVisibility{organization.OrganizationScope{OrganizationIDs:[]uint{visible.ID}}},nil,nil,platformdatabase.NewTransactionRunner(db))
 tree,err:=service.Tree(ctx,7)
 if err!=nil||len(tree)!=1||tree[0].ID!=root.ID||tree[0].Manageable||len(tree[0].Children)!=1||tree[0].Children[0].ID!=visible.ID||!tree[0].Children[0].Manageable{t.Fatalf("scoped tree = %#v, %v",tree,err)}
 if _,err:=service.Users(ctx,7,root.ID);err==nil{t.Fatal("positioning ancestor exposed members")}
 parent:=uint(0)
 if _,err:=service.UpdateUnit(ctx,7,root.ID,organization.UpdateUnitRequest{ParentID:&parent});err==nil{t.Fatal("positioning ancestor was moved")}
 if err:=service.SetUsers(ctx,7,root.ID,[]uint{});err==nil{t.Fatal("positioning ancestor membership changed")}
 all:=organization.NewService(repository,organization.NewHierarchy(repository),membershipVisibility{organization.OrganizationScope{All:true}},nil,nil,platformdatabase.NewTransactionRunner(db))
 tree,err=all.Tree(ctx,7)
 if err!=nil||len(tree)!=1||!tree[0].Manageable||len(tree[0].Children)!=2||!tree[0].Children[0].Manageable||!tree[0].Children[1].Manageable{t.Fatalf("all tree = %#v, %v",tree,err)}
}

func TestUserMembershipsRejectProtectedSelfAndInvalidOrganizationsAtomically(t *testing.T){
 db:=testutil.OpenIsolatedSQLite(t)
 if err:=db.AutoMigrate(append(organization.Models(),&authgorm.UserAccessVersion{})...);err!=nil{t.Fatal(err)}
 repository:=organization.NewGORMRepository(db);ctx:=context.Background()
 visible:=organization.Unit{Name:"Visible",Code:"visible",Status:1};outside:=organization.Unit{Name:"Outside",Code:"outside",Status:1}
 for _,unit:=range []*organization.Unit{&visible,&outside}{if err:=repository.CreateUnit(ctx,unit);err!=nil{t.Fatal(err)}}
 joined:=time.Date(2026,8,1,2,3,4,0,time.UTC)
 if err:=db.Create(&organization.Membership{UserID:20,OrganizationID:visible.ID,CreatedAt:joined}).Error;err!=nil{t.Fatal(err)}
 versions:=authgorm.NewAccessVersions(db);if _,err:=versions.Ensure(ctx,20);err!=nil{t.Fatal(err)}
 normal:=organization.ManagedUser{Exists:true,Visible:true}
 for _,test:=range []struct{name string;operator uint;user organization.ManagedUser;ids []uint;expected int;code organization.ErrorCode}{
  {"protected",7,organization.ManagedUser{Exists:true,Visible:true,Protected:true},[]uint{},1,organization.CodeConflict},
  {"self",20,normal,[]uint{},1,organization.CodeValidationInvalid},
  {"outside user",7,organization.ManagedUser{Exists:true},[]uint{},1,organization.CodePermissionDenied},
  {"missing user",7,organization.ManagedUser{},[]uint{},1,organization.CodeNotFound},
  {"outside organization",7,normal,[]uint{outside.ID},1,organization.CodePermissionDenied},
  {"missing organization",7,normal,[]uint{999},1,organization.CodeValidationInvalid},
  {"zero organization",7,normal,[]uint{0},1,organization.CodeValidationInvalid},
  {"missing version",7,normal,[]uint{},0,organization.CodeValidationInvalid},
 }{t.Run(test.name,func(t *testing.T){
  service:=organization.NewUserMembershipService(repository,membershipVisibility{organization.OrganizationScope{OrganizationIDs:[]uint{visible.ID}}},membershipTargetPolicy{test.user},versions,platformdatabase.NewTransactionRunner(db))
  _,err:=service.Set(ctx,test.operator,20,organization.UpdateUserOrganizationsRequest{OrganizationIDs:test.ids,ExpectedAccessVersion:test.expected})
  if code,_:=organization.CodeOf(err);code!=test.code{t.Fatalf("error = %v, want %s",err,test.code)}
  retained,err:=repository.MemberOrganizationMemberships(ctx,20)
  if err!=nil||len(retained)!=1||retained[0].OrganizationID!=visible.ID||!retained[0].JoinedAt.Equal(joined){t.Fatalf("rejected memberships = %#v, %v",retained,err)}
  version,err:=versions.Current(ctx,20);if err!=nil||version!=1{t.Fatalf("rejected version = %d, %v",version,err)}
 })}
}
