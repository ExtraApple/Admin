package initialize

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"admin/model"
)

func TestAccessVersionTransitionComponentsAreRetired(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..")

	retiredPaths := []string{
		"initialize/access_version_migration.go",
		"initialize/mysql_named_lock.go",
		"initialize/access_version_deployment.go",
		"service/access_version_consistency.go",
		"model/access_version_migration_state.go",
		"cmd/access-version-qualification",
		"internal/accessversionqualification",
		"cmd/access-version-drop-legacy-column",
		"initialize/testdata/access_version_migration_users.sql",
		"initialize/testutil/run-access-version-lifecycle-e2e.ps1",
		"initialize/testutil/run-access-version-rollback-preflight.ps1",
		"initialize/access_version_migration_fixture_test.go",
		"initialize/access_version_migration_mysql_integration_test.go",
		"initialize/access_version_deployment_test.go",
		"initialize/access_version_switch_runbook_test.go",
		"initialize/access_version_accelerated_qualification_policy_test.go",
		"initialize/access_version_pre_task_observation_policy_test.go",
		"service/access_version_consistency_test.go",
		"service/access_version_rollback_preflight_mysql_integration_test.go",
		"service/access_version_legacy_test.go",
		"router/legacy_access_version_test.go",
		"service/access_version_metrics_snapshot_test.go",
		"initialize/access_version_post_drop_mysql_integration_test.go",
		"initialize/access_version_final_documentation_test.go",
	}
	for _, retiredPath := range retiredPaths {
		absolutePath := filepath.Join(repoRoot, filepath.FromSlash(retiredPath))
		if _, err := os.Stat(absolutePath); err == nil {
			t.Errorf("retired access-version transition path still exists: %s", retiredPath)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect retired path %s: %v", retiredPath, err)
		}
	}

	evidenceTests, err := filepath.Glob(
		filepath.Join(
			repoRoot,
			"initialize",
			"access_version_*_evidence_test.go",
		),
	)
	if err != nil {
		t.Fatalf("find retired access-version evidence tests: %v", err)
	}
	for _, evidenceTest := range evidenceTests {
		t.Errorf(
			"retired access-version evidence test still exists: %s",
			filepath.Base(evidenceTest),
		)
	}

	for _, registeredModel := range model.Models {
		modelType := reflect.TypeOf(registeredModel)
		if modelType.Kind() == reflect.Pointer {
			modelType = modelType.Elem()
		}
		if modelType.Name() == "AccessVersionMigrationState" {
			t.Error("model.Models still registers AccessVersionMigrationState")
		}
	}

	metricsSource, err := os.ReadFile(
		filepath.Join(repoRoot, "service", "access_version_metrics.go"),
	)
	if err != nil {
		t.Fatalf("read access-version metrics source: %v", err)
	}
	for _, retiredField := range []string{
		"MirrorWriteFailures",
		"ConsistencyDifferences",
	} {
		if strings.Contains(string(metricsSource), retiredField) {
			t.Errorf(
				"AccessVersionMetricsSnapshot still exposes retired field %s",
				retiredField,
			)
		}
	}

	accessVersionSource, err := os.ReadFile(
		filepath.Join(repoRoot, "service", "access_version.go"),
	)
	if err != nil {
		t.Fatalf("read access-version repository source: %v", err)
	}
	if strings.Contains(
		string(accessVersionSource),
		"ErrAccessVersionMirrorFailed",
	) {
		t.Error("access-version repository still exposes retired mirror-write error")
	}
}
