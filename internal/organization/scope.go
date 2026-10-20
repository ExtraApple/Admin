package organization

import "context"

// ScopeReader exposes only organization facts needed by another module's
// caller-owned authorization contract. It does not expose GORM or model types.
type ScopeReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	CurrentMemberOrganizationIDs(context.Context, uint) ([]uint, error)
	Memberships(context.Context, uint) ([]MembershipFact, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	ExistingOrganizationIDs(context.Context, []uint) ([]uint, error)
	AllOrganizationIDs(context.Context) ([]uint, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
	CurrentMemberUserIDs(context.Context, uint) ([]uint, error)
	CurrentMemberUserIDsByOrganizations(context.Context, []uint) ([]uint, error)
	WithLockedHierarchy(context.Context, func(context.Context) error) error
}

type scopeReader struct {
	repository Repository
	hierarchy  HierarchyReader
}

func NewScopeReader(repository Repository, hierarchy HierarchyReader) ScopeReader {
	return &scopeReader{repository: repository, hierarchy: hierarchy}
}

func (reader *scopeReader) MemberOrganizationIDs(ctx context.Context, userID uint) ([]uint, error) {
	return reader.hierarchy.MemberOrganizationIDs(ctx, userID)
}

func (reader *scopeReader) CurrentMemberOrganizationIDs(ctx context.Context, userID uint) ([]uint, error) {
	return reader.repository.CurrentMemberOrganizationIDs(ctx, userID)
}

func (reader *scopeReader) Memberships(ctx context.Context, userID uint) ([]MembershipFact, error) {
	return reader.repository.MemberOrganizationMemberships(ctx, userID)
}

func (reader *scopeReader) DescendantOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	return reader.hierarchy.DescendantOrganizationIDs(ctx, ids)
}

// WithLockedHierarchy joins the caller's transaction and reuses its locked
// current parent graph for existence and scope decisions in the callback.
func (reader *scopeReader) WithLockedHierarchy(ctx context.Context, operation func(context.Context) error) error {
	parents, err := reader.repository.LockOrganizationParents(ctx)
	if err != nil {
		return err
	}
	return operation(withOrganizationParents(ctx, parents))
}

func (reader *scopeReader) ExistingOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return []uint{}, nil
	}
	if parents, locked := ctx.Value(organizationParentsContextKey{}).([]OrganizationParent); locked {
		selected := make(map[uint]struct{}, len(ids))
		for _, id := range ids {
			selected[id] = struct{}{}
		}
		result := make([]uint, 0, len(ids))
		for _, parent := range parents {
			if _, exists := selected[parent.ID]; exists {
				result = append(result, parent.ID)
			}
		}
		return result, nil
	}
	units, err := reader.repository.ListUnitsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]uint, len(units))
	for index := range units {
		result[index] = units[index].ID
	}
	return uniqueIDs(result), nil
}

func (reader *scopeReader) AllOrganizationIDs(ctx context.Context) ([]uint, error) {
	units, err := reader.repository.ListAllUnits(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, len(units))
	for index := range units {
		ids[index] = units[index].ID
	}
	return uniqueIDs(ids), nil
}

func (reader *scopeReader) MemberUserIDs(ctx context.Context, organizationID uint) ([]uint, error) {
	return reader.repository.MemberUserIDs(ctx, organizationID)
}

func (reader *scopeReader) CurrentMemberUserIDs(ctx context.Context, organizationID uint) ([]uint, error) {
	return reader.repository.CurrentMemberUserIDs(ctx, organizationID)
}

func (reader *scopeReader) CurrentMemberUserIDsByOrganizations(ctx context.Context, organizationIDs []uint) ([]uint, error) {
	return reader.repository.CurrentMemberUserIDsByOrganizations(ctx, organizationIDs)
}
