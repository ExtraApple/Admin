package application

import (
	"context"
	"strings"

	"admin/internal/platform/configimpact"
)

func (service *Service) verifyAPIFingerprint(ctx context.Context, expected string, input ImpactPreviewInput) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	if !configimpact.ValidFingerprint(expected) {
		return NewError(CodeValidationInvalid, nil)
	}
	plan, err := service.buildImpactPlan(ctx, input)
	if err != nil {
		return err
	}
	fingerprint, err := plan.Fingerprint()
	if err != nil {
		return NewError(CodeInternalError, err)
	}
	if fingerprint != expected {
		return NewError(CodeConflict, nil)
	}
	return nil
}

func optionalAPIFingerprint(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
