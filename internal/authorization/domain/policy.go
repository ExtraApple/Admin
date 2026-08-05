package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type DataScope string

const (
	DataScopeAll            DataScope = "all"
	DataScopeSelf           DataScope = "self"
	DataScopeOrg            DataScope = "org"
	DataScopeOrgAndChildren DataScope = "org_and_children"
	DataScopeCustom         DataScope = "custom"
)

func ParseDataScope(value string) (DataScope, error) {
	if value == "" {
		return DataScopeAll, nil
	}
	scope := DataScope(value)
	switch scope {
	case DataScopeAll, DataScopeSelf, DataScopeOrg, DataScopeOrgAndChildren, DataScopeCustom:
		return scope, nil
	default:
		return "", fmt.Errorf("数据范围不合法")
	}
}

func IsProtectedRole(code string) bool { return strings.TrimSpace(code) == "admin" }

type PermissionCode struct{ value string }

func NewPermissionCode(value string) (PermissionCode, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 100 {
		return PermissionCode{}, errors.New("权限码不合法")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return PermissionCode{}, errors.New("权限码不合法")
		}
	}
	return PermissionCode{value: value}, nil
}

func (code PermissionCode) String() string { return code.value }

type Principal struct{ UserID uint }

type AccessSnapshot struct {
	Principal   Principal
	Roles       []string
	Permissions []string
	Version     int
}

func (snapshot AccessSnapshot) IsAdmin() bool {
	for _, role := range snapshot.Roles {
		if IsProtectedRole(role) {
			return true
		}
	}
	return false
}

type UserScope struct {
	All     bool
	UserIDs []uint
}

func AllUsersScope() UserScope { return UserScope{All: true} }

func UserScopeForSelf(userID uint) UserScope {
	if userID == 0 {
		return UserScope{}
	}
	return UserScope{UserIDs: []uint{userID}}
}

func UserScopeForIDs(ids []uint) UserScope { return UserScope{UserIDs: uniqueIDs(ids)} }

type OrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}

func AllOrganizationsScope() OrganizationScope { return OrganizationScope{All: true} }

func OrganizationScopeForSelf() OrganizationScope { return OrganizationScope{} }

func OrganizationScopeForIDs(ids []uint) OrganizationScope {
	return OrganizationScope{OrganizationIDs: uniqueIDs(ids)}
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
