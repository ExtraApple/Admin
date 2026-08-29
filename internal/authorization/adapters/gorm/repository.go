package gormadapter

import (
	"context"
	"errors"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *Repository) ListRoles(ctx context.Context, offset, limit int) ([]domain.Role, int64, error) {
	db := repository.connection(ctx)
	var total int64
	if err := db.Model(&Role{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []Role
	if err := db.Order("sort asc, id asc").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	result := make([]domain.Role, len(records))
	for index := range records {
		result[index] = roleToDomain(records[index])
	}
	return result, total, nil
}

func (repository *Repository) FindRole(ctx context.Context, roleID uint) (domain.Role, error) {
	var record Role
	if err := repository.connection(ctx).First(&record, roleID).Error; err != nil {
		return domain.Role{}, mapNotFound(err)
	}
	return roleToDomain(record), nil
}

func (repository *Repository) RoleNameExists(ctx context.Context, name string, exceptID uint) (bool, error) {
	return repository.roleFieldExists(ctx, "name", name, exceptID)
}

func (repository *Repository) RoleCodeExists(ctx context.Context, code string, exceptID uint) (bool, error) {
	return repository.roleFieldExists(ctx, "code", code, exceptID)
}

func (repository *Repository) roleFieldExists(ctx context.Context, field, value string, exceptID uint) (bool, error) {
	query := repository.connection(ctx).Model(&Role{}).Where(field+" = ?", value)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repository *Repository) CreateRole(ctx context.Context, role *domain.Role) error {
	record := roleFromDomain(*role)
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	role.ID = record.ID
	return nil
}

func (repository *Repository) UpdateRole(ctx context.Context, role domain.Role) error {
	return repository.connection(ctx).Model(&Role{}).Where("id = ?", role.ID).Updates(map[string]any{
		"name": role.Name, "code": role.Code, "description": role.Description,
		"sort": role.Sort, "status": role.Status, "data_scope": string(role.DataScope),
	}).Error
}

func (repository *Repository) DeleteRole(ctx context.Context, roleID uint) error {
	db := repository.connection(ctx)
	if err := db.Where("role_id = ?", roleID).Delete(&UserRole{}).Error; err != nil {
		return err
	}
	if err := db.Where("role_id = ?", roleID).Delete(&RolePermission{}).Error; err != nil {
		return err
	}
	if err := db.Where("role_id = ?", roleID).Delete(&RoleDataScope{}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&Role{}, roleID).Error
}

func (repository *Repository) UserIDsByRole(ctx context.Context, roleID uint) ([]uint, error) {
	var ids []uint
	if err := repository.connection(ctx).Model(&UserRole{}).Where("role_id = ?", roleID).Order("user_id asc").Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return nonNilIDs(ids), nil
}

func (repository *Repository) RoleIDsByUser(ctx context.Context, userID uint) ([]uint, error) {
	var roleIDs []uint
	if err := repository.connection(ctx).Model(&UserRole{}).Where("user_id = ?", userID).Order("role_id asc").Pluck("role_id", &roleIDs).Error; err != nil {
		return nil, err
	}
	return nonNilIDs(roleIDs), nil
}

func (repository *Repository) ReplaceRoleUsers(ctx context.Context, roleID uint, userIDs []uint) error {
	db := repository.connection(ctx)
	if err := db.Where("role_id = ?", roleID).Delete(&UserRole{}).Error; err != nil {
		return err
	}
	ids := uniqueIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	records := make([]UserRole, len(ids))
	for index, userID := range ids {
		records[index] = UserRole{UserID: userID, RoleID: roleID}
	}
	return db.Create(&records).Error
}

func (repository *Repository) ReplaceRoleDataScope(ctx context.Context, roleID uint, scope domain.DataScope, organizationIDs []uint) error {
	db := repository.connection(ctx)
	if err := db.Model(&Role{}).Where("id = ?", roleID).Update("data_scope", string(scope)).Error; err != nil {
		return err
	}
	if err := db.Where("role_id = ?", roleID).Delete(&RoleDataScope{}).Error; err != nil {
		return err
	}
	ids := uniqueIDs(organizationIDs)
	if scope != domain.DataScopeCustom || len(ids) == 0 {
		return nil
	}
	records := make([]RoleDataScope, len(ids))
	for index, organizationID := range ids {
		records[index] = RoleDataScope{RoleID: roleID, OrganizationID: organizationID}
	}
	return db.Create(&records).Error
}

func (repository *Repository) RoleDataScope(ctx context.Context, roleID uint) (domain.RoleDataScope, error) {
	role, err := repository.FindRole(ctx, roleID)
	if err != nil {
		return domain.RoleDataScope{}, err
	}
	ids, err := repository.CustomOrganizationIDs(ctx, roleID)
	if err != nil {
		return domain.RoleDataScope{}, err
	}
	return domain.RoleDataScope{RoleID: roleID, DataScope: role.DataScope, OrganizationIDs: ids}, nil
}

func (repository *Repository) RolesForUser(ctx context.Context, userID uint) ([]domain.Role, error) {
	var records []Role
	if err := repository.connection(ctx).Model(&Role{}).
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ? AND roles.status = ?", userID, 1).
		Order("roles.sort asc, roles.id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Role, len(records))
	for index := range records {
		result[index] = roleToDomain(records[index])
	}
	return result, nil
}

func (repository *Repository) CustomOrganizationIDs(ctx context.Context, roleID uint) ([]uint, error) {
	var ids []uint
	if err := repository.connection(ctx).Model(&RoleDataScope{}).Where("role_id = ?", roleID).Order("organization_id asc").Pluck("organization_id", &ids).Error; err != nil {
		return nil, err
	}
	return nonNilIDs(ids), nil
}
func (repository *Repository) UserIDsByRoleIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	if len(roleIDs) == 0 {
		return []uint{}, nil
	}
	var ids []uint
	err := repository.connection(ctx).Model(&UserRole{}).Distinct("user_id").Where("role_id IN ?", roleIDs).Order("user_id asc").Pluck("user_id", &ids).Error
	return nonNilIDs(ids), err
}

func (repository *Repository) RoleIDsByPermissionCode(ctx context.Context, code string) ([]uint, error) {
	var ids []uint
	err := repository.connection(ctx).Model(&RolePermission{}).
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("permissions.code = ?", code).Order("role_permissions.role_id asc").Pluck("role_permissions.role_id", &ids).Error
	return nonNilIDs(ids), err
}

func (repository *Repository) LockPermissionByCode(ctx context.Context, code string) (domain.Permission, bool, error) {
	var record Permission
	err := repository.connection(ctx).Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", code).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Permission{}, false, nil
	}
	if err != nil {
		return domain.Permission{}, false, err
	}
	permission, err := permissionToDomain(record)
	return permission, true, err
}

func (repository *Repository) EnsurePermissionByCode(ctx context.Context, code, name, group string, sort int) (domain.Permission, bool, error) {
	permissionCode, err := domain.NewPermissionCode(code)
	if err != nil {
		return domain.Permission{}, false, err
	}
	permission, found, err := repository.LockPermissionByCode(ctx, permissionCode.String())
	if err != nil {
		return domain.Permission{}, false, err
	}
	if found {
		var record Permission
		if err := repository.connection(ctx).Unscoped().Where("code = ?", permissionCode.String()).First(&record).Error; err != nil {
			return domain.Permission{}, false, err
		}
		if record.DeletedAt.Valid {
			if err := repository.connection(ctx).Unscoped().Model(&record).Updates(map[string]any{"deleted_at": nil}).Error; err != nil {
				return domain.Permission{}, false, err
			}
			permission, _ = permissionToDomain(record)
		}
		return permission, false, nil
	}
	permission = domain.Permission{Name: name, Code: permissionCode, Group: group, Sort: sort}
	if err := repository.CreatePermission(ctx, &permission); err != nil {
		return domain.Permission{}, false, err
	}
	return permission, true, nil
}

func (repository *Repository) MergePermissionRoles(ctx context.Context, fromPermissionID, toPermissionID uint) error {
	db := repository.connection(ctx)
	var roles []RolePermission
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("permission_id = ?", fromPermissionID).Find(&roles).Error; err != nil {
		return err
	}
	for _, role := range roles {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&RolePermission{RoleID: role.RoleID, PermissionID: toPermissionID}).Error; err != nil {
			return err
		}
	}
	return db.Where("permission_id = ?", fromPermissionID).Delete(&RolePermission{}).Error
}

func (repository *Repository) PermissionHasRoleReferences(ctx context.Context, permissionID uint) (bool, error) {
	var count int64
	err := repository.connection(ctx).Model(&RolePermission{}).Where("permission_id = ?", permissionID).Count(&count).Error
	return count > 0, err
}

func (repository *Repository) DeletePermissionForNavigation(ctx context.Context, permissionID uint) error {
	db := repository.connection(ctx)
	if err := db.Where("permission_id = ?", permissionID).Delete(&RolePermission{}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&Permission{}, permissionID).Error
}

func (repository *Repository) ListPermissions(ctx context.Context, offset, limit int) ([]domain.Permission, int64, error) {
	db := repository.connection(ctx)
	var total int64
	if err := db.Model(&Permission{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []Permission
	if err := db.Order("sort asc, id asc").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	permissions, _, err := permissionsToDomain(records)
	return permissions, total, err
}

func (repository *Repository) FindPermission(ctx context.Context, permissionID uint) (domain.Permission, error) {
	var record Permission
	if err := repository.connection(ctx).First(&record, permissionID).Error; err != nil {
		return domain.Permission{}, mapNotFound(err)
	}
	return permissionToDomain(record)
}

func (repository *Repository) PermissionCodeExists(ctx context.Context, code string) (bool, error) {
	var count int64
	if err := repository.connection(ctx).Model(&Permission{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repository *Repository) CreatePermission(ctx context.Context, permission *domain.Permission) error {
	record := permissionFromDomain(*permission)
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	permission.ID = record.ID
	return nil
}

func (repository *Repository) UpdatePermission(ctx context.Context, permission domain.Permission) error {
	return repository.connection(ctx).Model(&Permission{}).Where("id = ?", permission.ID).Updates(map[string]any{
		"name": permission.Name, "group": permission.Group, "sort": permission.Sort,
	}).Error
}

func (repository *Repository) DeletePermission(ctx context.Context, permissionID uint) error {
	db := repository.connection(ctx)
	if err := db.Where("permission_id = ?", permissionID).Delete(&RolePermission{}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&Permission{}, permissionID).Error
}

func (repository *Repository) RoleIDsByPermission(ctx context.Context, permissionID uint) ([]uint, error) {
	var ids []uint
	if err := repository.connection(ctx).Model(&RolePermission{}).Where("permission_id = ?", permissionID).Order("role_id asc").Pluck("role_id", &ids).Error; err != nil {
		return nil, err
	}
	return nonNilIDs(ids), nil
}

func (repository *Repository) ReplaceRolePermissions(ctx context.Context, roleID uint, permissionIDs []uint) error {
	db := repository.connection(ctx)
	if err := db.Where("role_id = ?", roleID).Delete(&RolePermission{}).Error; err != nil {
		return err
	}
	ids := uniqueIDs(permissionIDs)
	if len(ids) == 0 {
		return nil
	}
	records := make([]RolePermission, len(ids))
	for index, permissionID := range ids {
		records[index] = RolePermission{RoleID: roleID, PermissionID: permissionID}
	}
	return db.Create(&records).Error
}

func (repository *Repository) PermissionsByRole(ctx context.Context, roleID uint) ([]domain.Permission, error) {
	var records []Permission
	if err := repository.connection(ctx).Model(&Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.sort asc, permissions.id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	permissions, _, err := permissionsToDomain(records)
	return permissions, err
}

func (repository *Repository) PermissionCodesForUser(ctx context.Context, userID uint) ([]string, error) {
	var codes []string
	if err := repository.connection(ctx).Model(&Permission{}).
		Distinct("permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN roles ON roles.id = role_permissions.role_id AND roles.deleted_at IS NULL AND roles.status = 1").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Order("permissions.code asc").Pluck("permissions.code", &codes).Error; err != nil {
		return nil, err
	}
	return nonNilStrings(codes), nil
}

func (repository *Repository) AllPermissionCodes(ctx context.Context) ([]string, error) {
	var codes []string
	if err := repository.connection(ctx).Model(&Permission{}).Order("`group` asc, sort asc, id asc").Pluck("code", &codes).Error; err != nil {
		return nil, err
	}
	return nonNilStrings(codes), nil
}

func (repository *Repository) ListPermissionGroups(ctx context.Context, offset, limit int) ([]domain.PermissionGroup, int64, error) {
	db := repository.connection(ctx)
	var total int64
	if err := db.Model(&PermissionGroup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []PermissionGroup
	if err := db.Order("sort asc, id asc").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	result := make([]domain.PermissionGroup, len(records))
	for index, record := range records {
		result[index] = domain.PermissionGroup{ID: record.ID, Name: record.Name, Sort: record.Sort}
	}
	return result, total, nil
}

func (repository *Repository) FindPermissionGroup(ctx context.Context, groupID uint) (domain.PermissionGroup, error) {
	var record PermissionGroup
	if err := repository.connection(ctx).First(&record, groupID).Error; err != nil {
		return domain.PermissionGroup{}, mapNotFound(err)
	}
	return domain.PermissionGroup{ID: record.ID, Name: record.Name, Sort: record.Sort}, nil
}

func (repository *Repository) PermissionGroupNameExists(ctx context.Context, name string, exceptID uint) (bool, error) {
	query := repository.connection(ctx).Model(&PermissionGroup{}).Where("name = ?", name)
	if exceptID != 0 {
		query = query.Where("id != ?", exceptID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repository *Repository) CreatePermissionGroup(ctx context.Context, group *domain.PermissionGroup) error {
	record := PermissionGroup{Name: group.Name, Sort: group.Sort}
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return err
	}
	group.ID = record.ID
	return nil
}

func (repository *Repository) UpdatePermissionGroup(ctx context.Context, group domain.PermissionGroup) error {
	return repository.connection(ctx).Model(&PermissionGroup{}).Where("id = ?", group.ID).Updates(map[string]any{"name": group.Name, "sort": group.Sort}).Error
}

func (repository *Repository) DeletePermissionGroup(ctx context.Context, groupID uint) error {
	return repository.connection(ctx).Unscoped().Delete(&PermissionGroup{}, groupID).Error
}

func roleToDomain(record Role) domain.Role {
	scope, err := domain.ParseDataScope(record.DataScope)
	if err != nil {
		scope = domain.DataScopeAll
	}
	return domain.Role{ID: record.ID, Name: record.Name, Code: record.Code, Description: record.Description, Sort: record.Sort, Status: record.Status, DataScope: scope}
}

func roleFromDomain(role domain.Role) Role {
	return Role{Name: role.Name, Code: role.Code, Description: role.Description, Sort: role.Sort, Status: role.Status, DataScope: string(role.DataScope)}
}

func permissionToDomain(record Permission) (domain.Permission, error) {
	code, err := domain.NewPermissionCode(record.Code)
	if err != nil {
		return domain.Permission{}, err
	}
	return domain.Permission{ID: record.ID, Name: record.Name, Code: code, Group: record.Group, Sort: record.Sort}, nil
}

func permissionFromDomain(permission domain.Permission) Permission {
	return Permission{Name: permission.Name, Code: permission.Code.String(), Group: permission.Group, Sort: permission.Sort}
}

func permissionsToDomain(records []Permission) ([]domain.Permission, int64, error) {
	result := make([]domain.Permission, len(records))
	for index, record := range records {
		permission, err := permissionToDomain(record)
		if err != nil {
			return nil, 0, err
		}
		result[index] = permission
	}
	return result, int64(len(records)), nil
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func uniqueIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func nonNilIDs(ids []uint) []uint {
	if ids == nil {
		return []uint{}
	}
	return ids
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
