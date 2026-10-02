package application

import (
	"context"
	"strings"

	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
)

const authorizationOverviewRiskLimit = 10

type RiskKind string

const (
	RiskKindDisabledAssignedMenu       RiskKind = "disabled_assigned_menu"
	RiskKindMissingMenuPermission      RiskKind = "missing_menu_permission"
	RiskKindUsedRoleWithoutPermissions RiskKind = "used_role_without_permissions"
	RiskKindManagedUserWithoutRole     RiskKind = "managed_user_without_role"
)

type RiskResource string

type AuthorizationOverviewScope struct {
	DataScope         string                   `json:"data_scope"`
	OrganizationCount int                      `json:"organization_count"`
	User              domain.UserScope         `json:"-"`
	Organization      domain.OrganizationScope `json:"-"`
}

const (
	RiskResourceRole RiskResource = "role"
	RiskResourceUser RiskResource = "user"
)

type SessionPolicy struct {
	RevalidateOnNextRequest bool `json:"revalidate_on_next_request"`
}

type AuthorizationOverviewCount struct {
	Total       int `json:"total"`
	Enabled     int `json:"enabled"`
	Used        int `json:"used,omitempty"`
	WithoutRole int `json:"without_role,omitempty"`
	Manageable  int `json:"manageable,omitempty"`
}

type AuthorizationOverviewSummary struct {
	Roles         AuthorizationOverviewCount `json:"roles"`
	Users         AuthorizationOverviewCount `json:"users"`
	Organizations AuthorizationOverviewCount `json:"organizations"`
}

type AuthorizationRisk struct {
	Resource                         RiskResource  `json:"resource"`
	ResourceID                       uint          `json:"resource_id"`
	ResourceName                     string        `json:"resource_name"`
	IssueCount                       int           `json:"issue_count"`
	IssueKinds                       []RiskKind    `json:"issue_kinds"`
	PotentiallyAffectedUsers         int           `json:"potentially_affected_users"`
	PotentiallyAffectedOrganizations int           `json:"potentially_affected_organizations"`
	SessionPolicy                    SessionPolicy `json:"session_policy"`
}

type AuthorizationRiskSummary struct {
	Items   []AuthorizationRisk `json:"items"`
	Total   int64               `json:"total"`
	Limit   int                 `json:"limit"`
	HasMore bool                `json:"has_more"`
}

type AuthorizationOverview struct {
	Scope   AuthorizationOverviewScope   `json:"scope"`
	Summary AuthorizationOverviewSummary `json:"summary"`
	Risks   AuthorizationRiskSummary     `json:"risks"`
}

type AuthorizationRiskPage struct {
	Scope AuthorizationOverviewScope `json:"scope"`
	List  []AuthorizationRisk        `json:"list"`
	Total int64                      `json:"total"`
	Page  int                        `json:"page"`
	Size  int                        `json:"size"`
}

type AuthorizationRiskListRequest struct {
	Page     int
	Size     int
	Kind     RiskKind
	Resource RiskResource
	Keyword  string
}

type AuthorizationOverviewRoleFact struct {
	ID              uint
	Name            string
	Code            string
	Status          int
	DataScope       string
	UserIDs         []uint
	MenuIDs         []uint
	PermissionCodes []string
}

type AuthorizationOverviewMenuFact struct {
	ID             uint
	Name           string
	Status         int
	PermissionCode string
}

type AuthorizationOverviewUserFact struct {
	User            identitydomain.DirectoryUser
	RoleIDs         []uint
	OrganizationIDs []uint
}

type AuthorizationOverviewOrganizationFact struct {
	ID         uint
	Name       string
	Manageable bool
}

type AuthorizationOverviewFacts struct {
	Roles         []AuthorizationOverviewRoleFact
	Menus         []AuthorizationOverviewMenuFact
	Users         []AuthorizationOverviewUserFact
	Organizations []AuthorizationOverviewOrganizationFact
}

type AuthorizationOverviewReader interface {
	ReadAuthorizationFacts(context.Context, AuthorizationOverviewScope) (AuthorizationOverviewFacts, error)
}

func validateAuthorizationRiskListRequest(request AuthorizationRiskListRequest) error {
	if request.Page < 1 || request.Size < 1 || request.Size > 100 {
		return NewError(CodeValidationInvalid, nil)
	}
	if request.Kind != "" {
		switch request.Kind {
		case RiskKindDisabledAssignedMenu, RiskKindMissingMenuPermission, RiskKindUsedRoleWithoutPermissions, RiskKindManagedUserWithoutRole:
		default:
			return NewError(CodeValidationInvalid, nil)
		}
	}
	if request.Resource != "" && request.Resource != RiskResourceRole && request.Resource != RiskResourceUser {
		return NewError(CodeValidationInvalid, nil)
	}
	return nil
}

func normalizeAuthorizationRiskListRequest(request AuthorizationRiskListRequest) AuthorizationRiskListRequest {
	request.Keyword = strings.TrimSpace(request.Keyword)
	return request
}

func (facts AuthorizationOverviewFacts) ensureArrays() AuthorizationOverviewFacts {
	if facts.Roles == nil {
		facts.Roles = []AuthorizationOverviewRoleFact{}
	}
	if facts.Menus == nil {
		facts.Menus = []AuthorizationOverviewMenuFact{}
	}
	if facts.Users == nil {
		facts.Users = []AuthorizationOverviewUserFact{}
	}
	if facts.Organizations == nil {
		facts.Organizations = []AuthorizationOverviewOrganizationFact{}
	}
	return facts
}
