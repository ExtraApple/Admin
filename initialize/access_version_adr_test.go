package initialize

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAccessVersionADRRecordsOwnershipCutoverAndExitOrder(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	adrPath := filepath.Join(
		filepath.Dir(currentFile),
		"..",
		"docs",
		"adr",
		"0004-authorization-owns-access-version-storage.md",
	)
	content, err := os.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("read access-version storage ADR: %v", err)
	}

	adr := string(content)
	required := []string{
		"# Authorization 拥有授权版本存储",
		"## 状态",
		"已接受",
		"## 决策",
		"`user_access_versions`",
		"唯一读取事实来源",
		"停机切换",
		"不得混跑",
		"先停止镜像写",
		"移除全部运行时读写",
		"User GORM Model",
		"AutoMigrate",
		"独立 DDL",
		"删除 `users.token_version`",
		"## 取舍",
	}
	for _, marker := range required {
		if !strings.Contains(adr, marker) {
			t.Errorf("access-version ADR is missing required marker %q", marker)
		}
	}

	exitOrder := []string{
		"先停止镜像写",
		"移除全部运行时读写",
		"User GORM Model",
		"AutoMigrate",
		"独立 DDL",
		"删除 `users.token_version`",
	}
	exitSectionStart := strings.Index(adr, "观察期结束后的退出顺序固定为：")
	if exitSectionStart == -1 {
		t.Fatal("access-version ADR is missing the exit-order section")
	}
	exitSection := adr[exitSectionStart:]
	lastIndex := -1
	for _, step := range exitOrder {
		index := strings.Index(exitSection, step)
		if index == -1 {
			continue
		}
		if index <= lastIndex {
			t.Errorf("access-version exit step %q is out of order", step)
		}
		lastIndex = index
	}
}
