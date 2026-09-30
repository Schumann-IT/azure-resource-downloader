package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFileAtomic writes data to path so that a reader sees either the previous
// file or the complete new one, never a truncated mix: the bytes go to a
// temporary file in the target's own directory (".<stem>-*<ext>", so a rename
// never crosses a filesystem), which is set to 0644 — CreateTemp creates 0600,
// and every export file is 0644 — and then renamed over path. The temporary
// file is removed on every failure. The directory must already exist: creating
// it is the caller's decision, because for some trees (drift/) an absent
// directory means something.
func WriteFileAtomic(path string, data []byte) error {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+stem+"-*"+ext)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to set file permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace %s: %w", base, err)
	}
	return nil
}
