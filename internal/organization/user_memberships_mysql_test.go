//go:build mysql_integration

package organization_test

import (
 "context"
 "fmt"
 "os"
 "testing"
 "time"

 authgorm "admin/internal/authorization/adapters/gorm"
 authapplication "admin/internal/authorization/application"
 authdomain "admin/internal/authorization/domain"
 identitygorm "admin/internal/identity/adapters/gorm"
 identityapplication "admin/internal/identity/application"
 identitydomain "admin/internal/identity/domain"
 "admin/internal/organization"
 platformdatabase "admin/internal/platform/database"
 mysqldriver "github.com/go-sql-driver/mysql"
 "gorm.io/driver/mysql"
 "gorm.io/gorm"
)

func TestMySQLRoleAndOrganizationWritesShareOneOptimisticVersion(t *testing.T){
 configuration,err:=mysqldriver.ParseDSN(os.Getenv("ADMIN_TEST_MYSQL_DSN"))
 if err!=nil||configuration.DBName==""{t.Fatal("ADMIN_TEST_MYSQL_DSN must identify a test database")}
 configuration.DBName=""
 server,err:=gorm.Open(mysql.Open(configuration.FormatDSN()),&gorm.Config{})
 if err!=nil{t.Fatal(err)}
 connection,err:=server.DB();if err!=nil{t.Fatal(err)};t.Cleanup(func(){_ = connection.Close()})
 databaseName:=fmt.Sprintf("workbench_race_%d",time.Now().UnixNano())
 if err:=server.Exec("CREATE DATABASE `"+databaseName+"`").Error;err!=nil{t.Fatal(err)}
 t.Cleanup(func(){if err:=server.Exec("DROP DATABASE `"+databaseName+"`").Error;err!=nil{t.Error(err)}})
 configuration.DBName=databaseName
 db,err:=gorm.Open(mysql.Open(configuration.FormatDSN()),&gorm.Config{NowFunc:func()time.Time{return time.Now().UTC()}})
 if err!=nil{t.Fatal(err)}
 sqlDB,err:=db.DB();if err!=nil{t.Fatal(err)};t.Cleanup(func(){_ = sqlDB.Close()})
 models:=append(authgorm.Models(),organization.Models()...);models=append(models,identitygorm.UserModel{})
 if err:=db.AutoMigrate(models...);err!=nil{t.Fatal(err)}
 ctx:=context.Background();transactions:=platformdatabase.NewTransactionRunner(db)
 users:=identitygorm.NewRepository(db);directory:=identityapplication.NewDirectoryService(users)
 manager:=identitydomain.User{Username:"manager",Email:"manager@example.test",Status:1}
 if err:=users.Create(ctx,&manager);err!=nil{t.Fatal(err)}
 roles:=authgorm.NewRepository(db)
 managerRole:=authdomain.Role{Name:"Manager",Code:"manager",Status:1,DataScope:authdomain.DataScopeAll}
 reader:=authdomain.Role{Name:"Reader",Code:"reader",Status:1,DataScope:authdomain.DataScopeSelf}
 for _,role:=range []*authdomain.Role{&managerRole,&reader}{if err:=roles.CreateRole(ctx,role);err!=nil{t.Fatal(err)}}
 if err:=roles.ReplaceUserRoles(ctx,manager.ID,[]uint{managerRole.ID});err!=nil{t.Fatal(err)}
 organizations:=organization.NewGORMRepository(db);unit:=organization.Unit{Name:"Team",Code:"team",Status:1}
 if err:=organizations.CreateUnit(ctx,&unit);err!=nil{t.Fatal(err)}
 versions:=authgorm.NewAccessVersions(db)
 authorization:=authapplication.NewService(roles,transactions,organization.NewScopeReader(organizations,organization.NewHierarchy(organizations)),directory,versions)
 memberships:=organization.NewUserMembershipService(organizations,membershipVisibility{organization.OrganizationScope{All:true}},membershipTargetPolicy{organization.ManagedUser{Exists:true,Visible:true}},versions,transactions)
 for _,initialized:=range []bool{false,true}{t.Run(fmt.Sprintf("version_row_exists_%t",initialized),func(t *testing.T){
  target:=identitydomain.User{Username:fmt.Sprintf("target-%t",initialized),Email:fmt.Sprintf("target-%t@example.test",initialized),Status:1}
  if err:=users.Create(ctx,&target);err!=nil{t.Fatal(err)}
  if initialized{if _,err:=versions.Ensure(ctx,target.ID);err!=nil{t.Fatal(err)}}
  type outcome struct{dimension string;version int;err error}
  start:=make(chan struct{});results:=make(chan outcome,2)
  go func(){<-start;version,err:=authorization.SetUserRoles(ctx,manager.ID,target.ID,authapplication.UpdateUserRolesRequest{RoleIDs:[]uint{reader.ID},ExpectedAccessVersion:1});results<-outcome{"roles",version,err}}()
  go func(){<-start;version,err:=memberships.Set(ctx,manager.ID,target.ID,organization.UpdateUserOrganizationsRequest{OrganizationIDs:[]uint{unit.ID},ExpectedAccessVersion:1});results<-outcome{"organizations",version,err}}()
  close(start)
  first,second:=<-results,<-results
  successful,conflict:=first,second;if first.err!=nil{successful,conflict=second,first}
  if successful.err!=nil||successful.version!=2||conflict.err==nil{t.Fatalf("concurrent outcomes = %#v / %#v",first,second)}
  if conflict.dimension=="roles"{if code,_:=authapplication.CodeOf(conflict.err);code!=authapplication.CodeConflict{t.Fatalf("role conflict = %v",conflict.err)}}else if code,_:=organization.CodeOf(conflict.err);code!=organization.CodeConflict{t.Fatalf("organization conflict = %v",conflict.err)}
  roleIDs,err:=roles.RoleIDsByUser(ctx,target.ID);if err!=nil{t.Fatal(err)}
  orgIDs,err:=organizations.MemberOrganizationIDs(ctx,target.ID);if err!=nil{t.Fatal(err)}
  if successful.dimension=="roles"{if len(roleIDs)!=1||roleIDs[0]!=reader.ID||len(orgIDs)!=0{t.Fatalf("partial race commit roles=%v organizations=%v",roleIDs,orgIDs)}}else if len(roleIDs)!=0||len(orgIDs)!=1||orgIDs[0]!=unit.ID{t.Fatalf("partial race commit roles=%v organizations=%v",roleIDs,orgIDs)}
  current,err:=versions.Current(ctx,target.ID);if err!=nil||current!=2{t.Fatalf("race version = %d, %v",current,err)}
 })}
}

func TestMySQLLockedVersionOverridesEarlierRepeatableReadSnapshot(t *testing.T){
 dsn:=os.Getenv("ADMIN_TEST_MYSQL_DSN")
 if dsn==""{t.Fatal("ADMIN_TEST_MYSQL_DSN is required")}
 db,err:=gorm.Open(mysql.Open(dsn),&gorm.Config{})
 if err!=nil{t.Fatal(err)}
 connection,err:=db.DB();if err!=nil{t.Fatal(err)};t.Cleanup(func(){_ = connection.Close()})
 if err:=db.AutoMigrate(&authgorm.UserAccessVersion{});err!=nil{t.Fatal(err)}
 targetID:=uint(time.Now().UnixNano()%1000000000)+1000000000
 versions:=authgorm.NewAccessVersions(db);ctx:=context.Background()
 t.Cleanup(func(){if err:=db.Delete(&authgorm.UserAccessVersion{},"user_id = ?",targetID).Error;err!=nil{t.Error(err)}})
 if _,err:=versions.Ensure(ctx,targetID);err!=nil{t.Fatal(err)}
 err=platformdatabase.NewTransactionRunner(db).RunRepeatableRead(ctx,func(tx context.Context)error{
  before,err:=versions.Current(tx,targetID);if err!=nil{return err};if before!=1{return fmt.Errorf("snapshot version = %d",before)}
  changed,err:=versions.EnsureAndIncrement(ctx,targetID);if err!=nil{return err};if changed!=2{return fmt.Errorf("changed version = %d",changed)}
  locked,err:=versions.Ensure(tx,targetID);if err!=nil{return err};if locked!=2{return fmt.Errorf("locked version = %d; must not reuse snapshot version 1",locked)}
  return nil
 })
 if err!=nil{t.Fatal(err)}
}
