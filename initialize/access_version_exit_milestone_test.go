package initialize

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestAccessVersionExitMilestoneIsSignedAndAuditable(t *testing.T) {
	t.Parallel()

	changeDir := accessVersionChangeDirectory(t)

	checklistPath := filepath.Join(changeDir, "acceptance-checklists.md")
	checklistContent, err := os.ReadFile(checklistPath)
	if err != nil {
		t.Fatalf("read access-version acceptance checklist: %v", err)
	}
	checklist := string(checklistContent)

	requiredChecklistMarkers := []string{
		"- [x] 文档与 ADR 已更新为新表唯一事实来源。",
		"退出里程碑状态：通过",
		"退出版本/Commit：当前工作区，未提交",
		"DDL 变更单：evidence/2026-07-31-legacy-column-drop/",
		"完成时间：2026-07-31",
		"负责人：Rog",
		"数据库复核人：本地自动化门禁",
		"安全/认证复核人：本地自动化门禁",
		"证据索引：evidence/2026-07-31-exit-milestone/README.md",
		"Change 是否可以关闭：是",
	}
	for _, marker := range requiredChecklistMarkers {
		if !strings.Contains(checklist, marker) {
			t.Errorf("exit milestone checklist is missing marker %q", marker)
		}
	}

	for _, placeholder := range []string{
		"退出里程碑状态：通过 / 失败",
		"Change 是否可以关闭：是 / 否",
	} {
		if strings.Contains(checklist, placeholder) {
			t.Errorf("exit milestone checklist still contains placeholder %q", placeholder)
		}
	}

	evidencePath := filepath.Join(
		changeDir,
		"evidence",
		"2026-07-31-exit-milestone",
		"README.md",
	)
	evidenceContent, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read exit milestone evidence index: %v", err)
	}
	evidence := string(evidenceContent)

	requiredEvidenceReferences := []string{
		"evidence/qualification/qualification-20260730T121335Z/RESULT.md",
		"evidence/2026-07-31-exit-version-validation/README.md",
		"evidence/2026-07-31-legacy-column-drop/README.md",
		"evidence/2026-07-31-post-drop-validation/README.md",
		"evidence/2026-07-31-transition-retirement/README.md",
		"final-validation.log",
	}
	for _, reference := range requiredEvidenceReferences {
		if !strings.Contains(evidence, reference) {
			t.Errorf("exit milestone evidence index is missing reference %q", reference)
		}
	}
}

func accessVersionChangeDirectory(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..")

	active := filepath.Join(
		repoRoot,
		"openspec",
		"changes",
		"migrate-access-version-storage",
	)
	if info, err := os.Stat(active); err == nil && info.IsDir() {
		return active
	}

	archived, err := filepath.Glob(filepath.Join(
		repoRoot,
		"openspec",
		"changes",
		"archive",
		"*-migrate-access-version-storage",
	))
	if err != nil {
		t.Fatalf("find archived access-version change: %v", err)
	}
	if len(archived) == 0 {
		t.Fatal("access-version change is neither active nor archived")
	}

	sort.Strings(archived)
	return archived[len(archived)-1]
}
