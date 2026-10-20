package organization

import "context"

type ManagedUser struct {
	Exists    bool
	Deleted   bool
	Visible   bool
	Protected bool
}
type UserManagementPolicy interface {
	ManagedUsers(context.Context, uint, []uint) (map[uint]ManagedUser, error)
}
type UserAccessVersions interface {
	Ensure(context.Context, uint) (int, error)
	EnsureAndIncrement(context.Context, uint) (int, error)
}
type UpdateUserOrganizationsRequest struct {
	OrganizationIDs       []uint
	ExpectedAccessVersion int
}
type UserOrganizationFact struct {
	UserID         uint
	OrganizationID uint
	Name           string
	Manageable     bool
}

// UserMembershipService performs a scoped delta, never a whole-organization replacement.
type UserMembershipService struct {
	repository   Repository
	visibility   VisibilityProvider
	policy       UserManagementPolicy
	versions     UserAccessVersions
	transactions TransactionRunner
}

func NewUserMembershipService(repository Repository, visibility VisibilityProvider, policy UserManagementPolicy, versions UserAccessVersions, transactions TransactionRunner) *UserMembershipService {
	return &UserMembershipService{repository: repository, visibility: visibility, policy: policy, versions: versions, transactions: transactions}
}
func (service *UserMembershipService) UserOrganizations(ctx context.Context, operatorID uint, userIDs []uint) ([]UserOrganizationFact, error) {
	scope, err := service.visibility.Scope(ctx, operatorID)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	facts, err := service.repository.UserOrganizationFacts(ctx, userIDs)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	for index := range facts {
		fact := &facts[index]
		fact.Manageable = scope.All || containsID(scope.OrganizationIDs, fact.OrganizationID)
		if !fact.Manageable {
			fact.OrganizationID = 0
			fact.Name = ""
		}
	}
	return facts, nil
}
func (service *UserMembershipService) Set(ctx context.Context, operatorID, targetID uint, request UpdateUserOrganizationsRequest) (int, error) {
	if operatorID == 0 || targetID == 0 || operatorID == targetID || request.ExpectedAccessVersion < 1 {
		return 0, NewError(CodeValidationInvalid, nil)
	}
	for _, id := range request.OrganizationIDs {
		if id == 0 {
			return 0, NewError(CodeValidationInvalid, nil)
		}
	}
	selected := uniqueIDs(request.OrganizationIDs)
	var result int
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		result = 0
		parents, err := service.repository.LockOrganizationParents(tx)
		if err != nil {
			return err
		}
		tx = withOrganizationParents(tx, parents)
		current, err := service.versions.Ensure(tx, targetID)
		if err != nil {
			return err
		}
		if current != request.ExpectedAccessVersion {
			return NewError(CodeConflict, nil)
		}
		users, err := service.policy.ManagedUsers(tx, operatorID, []uint{targetID})
		if err != nil {
			return err
		}
		user := users[targetID]
		if !user.Exists || user.Deleted {
			return NewError(CodeNotFound, nil)
		}
		if !user.Visible {
			return NewError(CodePermissionDenied, nil)
		}
		if user.Protected {
			return NewError(CodeConflict, nil)
		}
		scope, err := service.visibility.CurrentScope(tx, operatorID)
		if err != nil {
			return err
		}
		for _, id := range selected {
			if !organizationParentExists(parents, id) {
				return NewError(CodeValidationInvalid, nil)
			}
		}
		for _, id := range selected {
			if !scope.All && !containsID(scope.OrganizationIDs, id) {
				return NewError(CodePermissionDenied, nil)
			}
		}
		currentIDs, err := service.repository.CurrentMemberOrganizationIDs(tx, targetID)
		if err != nil {
			return err
		}
		manageableCurrent := currentIDs[:0]
		for _, id := range currentIDs {
			if (scope.All && organizationParentExists(parents, id)) || (!scope.All && containsID(scope.OrganizationIDs, id)) {
				manageableCurrent = append(manageableCurrent, id)
			}
		}
		added, removed := membershipDelta(manageableCurrent, selected)
		if len(added) == 0 && len(removed) == 0 {
			result = current
			return nil
		}
		if err := service.repository.ApplyUserMembershipDelta(tx, targetID, added, removed); err != nil {
			return err
		}
		result, err = service.versions.EnsureAndIncrement(tx, targetID)
		return err
	})
	if err != nil {
		if _, classified := CodeOf(err); classified {
			return 0, err
		}
		return 0, NewError(CodeInternalError, err)
	}
	return result, nil
}
