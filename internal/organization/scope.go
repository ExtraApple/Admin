package organization

import "context"

// ScopeReader exposes only organization facts needed by another module's
// caller-owned authorization contract. It does not expose GORM or model types.
type ScopeReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	ExistingOrganizationIDs(context.Context, []uint) ([]uint, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
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

func (reader *scopeReader) DescendantOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	return reader.hierarchy.DescendantOrganizationIDs(ctx, ids)
}

func (reader *scopeReader) ExistingOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	if len(ids) == 0 {
		return []uint{}, nil
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

func (reader *scopeReader) MemberUserIDs(ctx context.Context, organizationID uint) ([]uint, error) {
	return reader.repository.MemberUserIDs(ctx, organizationID)
}
