package app

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/organization"
)

type messagingScopeReaderProbe struct {
	organization.ScopeReader
	all          []uint
	descendants  []uint
	allCallCount int
}

func (probe *messagingScopeReaderProbe) AllOrganizationIDs(context.Context) ([]uint, error) {
	probe.allCallCount++
	return append([]uint(nil), probe.all...), nil
}

func (probe *messagingScopeReaderProbe) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return append([]uint(nil), probe.descendants...), nil
}

func TestMessagingOrganizationReaderForwardsOrganizationScopeFacets(t *testing.T) {
	probe := &messagingScopeReaderProbe{all: []uint{10, 20}, descendants: []uint{10, 11}}
	reader := messagingOrganizationReader{scope: probe}

	all, err := reader.AllOrganizationIDs(context.Background())
	if err != nil || !reflect.DeepEqual(all, []uint{10, 20}) || probe.allCallCount != 1 {
		t.Fatalf("AllOrganizationIDs() = %#v, %v, calls=%d", all, err, probe.allCallCount)
	}

	descendants, err := reader.DescendantOrganizationIDs(context.Background(), []uint{10})
	if err != nil || !reflect.DeepEqual(descendants, []uint{10, 11}) {
		t.Fatalf("DescendantOrganizationIDs() = %#v, %v", descendants, err)
	}
}

var _ organization.ScopeReader = (*messagingScopeReaderProbe)(nil)
