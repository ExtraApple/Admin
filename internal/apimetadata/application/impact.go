package application

import "admin/internal/platform/configimpact"

type ImpactOperation string

const (
	ImpactUpdateAPI          ImpactOperation = "update"
	ImpactDeleteAPI          ImpactOperation = "delete"
	ImpactSyncAPIs           ImpactOperation = "sync"
	ImpactSyncPermissions    ImpactOperation = "sync-permissions"
	ImpactGenerateMenuButton ImpactOperation = "generate-menu-button"
)

type ImpactPreview struct {
	Operation        ImpactOperation      `json:"operation"`
	NormalizedTarget any                  `json:"normalized_target"`
	Fingerprint      string               `json:"preview_fingerprint"`
	Summary          configimpact.Summary `json:"summary"`
}
