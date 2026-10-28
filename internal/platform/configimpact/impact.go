// Package configimpact contains shared, non-persistent configuration preview values.

package configimpact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Summary is the safe, non-credential impact projection shared by configuration
// preview endpoints. It intentionally contains counts and stable resource IDs,
// not user details, tokens, cookies, or storage errors.
type Summary struct {
	AffectedMenuIDs      []uint `json:"affected_menu_ids,omitempty"`
	AffectedAPIIDs       []uint `json:"affected_api_ids,omitempty"`
	AffectedRoleCount    int    `json:"affected_role_count"`
	AffectedUserCount    int    `json:"affected_user_count"`
	AccessVersionChange  bool   `json:"access_version_change"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
	CreatedCount         int    `json:"created_count,omitempty"`
	SkippedCount         int    `json:"skipped_count,omitempty"`
	ConflictCount        int    `json:"conflict_count,omitempty"`
}

// Plan is the normalized, read-only fact set shared by preview and commit
// paths. Domain modules populate NormalizedTarget and CurrentFacts after
// validating dependencies and calculating the affected-role/user summary.
type Plan struct {
	Operation        string
	NormalizedTarget any
	CurrentFacts     any
	Summary          Summary
}

func NewPlan(operation string, normalizedTarget, currentFacts any, summary Summary) (Plan, error) {
	if operation == "" {
		return Plan{}, ErrInvalidPlan
	}
	return Plan{
		Operation:        operation,
		NormalizedTarget: normalizedTarget,
		CurrentFacts:     currentFacts,
		Summary:          summary,
	}, nil
}

func (plan Plan) Fingerprint() (string, error) {
	return Fingerprint(struct {
		Operation        string `json:"operation"`
		NormalizedTarget any    `json:"normalized_target"`
		CurrentFacts     any    `json:"current_facts"`
	}{
		Operation:        plan.Operation,
		NormalizedTarget: plan.NormalizedTarget,
		CurrentFacts:     plan.CurrentFacts,
	})
}

var ErrInvalidPlan = errors.New("invalid configuration impact plan")

// Fingerprint returns a deterministic SHA-256 hex digest of a normalized
// operation fact. Callers must sort IDs and normalize all input fields before
// passing the value here.
func Fingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func ValidFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}
