// Package navigation owns menu configuration and menu/API binding behavior.

package navigation

import "admin/internal/platform/configimpact"

type ImpactOperation string

const (
	ImpactCreateMenu ImpactOperation = "create"
	ImpactUpdateMenu ImpactOperation = "update"
	ImpactDeleteMenu ImpactOperation = "delete"
	ImpactBindAPIs   ImpactOperation = "bind-apis"
	ImpactClearAPIs  ImpactOperation = "clear-apis"
	ImpactSyncMenus  ImpactOperation = "sync"
)

type ImpactPreview struct {
	Operation        ImpactOperation      `json:"operation"`
	NormalizedTarget any                  `json:"normalized_target"`
	Fingerprint      string               `json:"preview_fingerprint"`
	Summary          configimpact.Summary `json:"summary"`
}
