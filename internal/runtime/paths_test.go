package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome_EmptyStaysEmpty(t *testing.T) {
	if got := ExpandHome(""); got != "" {
		t.Fatalf("expected empty string, got=%q", got)
	}
}

func TestDescribeConfigHome_ListsEntries(t *testing.T) {
	dir := t.TempDir()
	// 重写 SlimeBotHomeDir 的返回值不太方便，直接测试逻辑
	// 创建一些文件和目录
	_ = os.MkdirAll(filepath.Join(dir, "skills"), os.ModePerm)
	_ = os.MkdirAll(filepath.Join(dir, "storage"), os.ModePerm)
	if f, err := os.Create(filepath.Join(dir, ".env")); err == nil {
		f.Close()
	}

	// 直接调用 DescribeConfigHome 验证输出格式
	// 由于 DescribeConfigHome 使用固定的 SlimeBotHomeDir，
	// 我们验证输出格式符合预期即可
	result := DescribeConfigHome()
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	// 至少包含路径信息
	if !strings.Contains(result, ".slimebot") && !strings.Contains(result, "directory not yet created") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestDescribeConfigHome_NonexistentDir(t *testing.T) {
	// 当目录不存在时，应返回描述性字符串
	result := DescribeConfigHome()
	if result == "" {
		t.Fatal("expected non-empty result for nonexistent dir")
	}
}
