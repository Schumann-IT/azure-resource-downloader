package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.yaml")

	if err := WriteFileAtomic(path, []byte("one\n")); err != nil {
		t.Fatalf("WriteFileAtomic() = %v", err)
	}
	if err := WriteFileAtomic(path, []byte("two\n")); err != nil {
		t.Fatalf("WriteFileAtomic() replace = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "two\n" {
		t.Fatalf("content = %q, %v; want the replacement", data, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".audit-") {
			t.Errorf("temp file %q left behind", e.Name())
		}
	}

	if err := WriteFileAtomic(filepath.Join(dir, "missing", "x.yaml"), []byte("x")); err == nil {
		t.Error("WriteFileAtomic into a missing directory must fail: creating it is the caller's decision")
	}
}
