package initialize

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAccessVersionExitRuntimeDoesNotReadOrWriteLegacyUserColumn(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..")

	files := map[string][]string{
		filepath.Join(repoRoot, "service", "access_version.go"): {
			"mirrorLegacyAccessVersion",
			"UpdateColumn(\"token_version\"",
		},
		filepath.Join(repoRoot, "initialize", "mysql.go"): {
			"runAccessVersionStorageMigrationLocked",
		},
		filepath.Join(repoRoot, "initialize", "admin.go"): {
			"TokenVersion:",
		},
		filepath.Join(repoRoot, "seed", "seed.go"): {
			"TokenVersion:",
		},
	}
	for path, forbidden := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read exit-version runtime file %s: %v", path, err)
			continue
		}
		document := string(content)
		for _, marker := range forbidden {
			if strings.Contains(document, marker) {
				t.Errorf(
					"exit-version runtime file %s still contains legacy access marker %q",
					path,
					marker,
				)
			}
		}
	}
}
