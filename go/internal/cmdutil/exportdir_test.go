package cmdutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedExport creates <base>/<domain>/resources/metadata.yaml.
func seedExport(t *testing.T, base, domain string) {
	t.Helper()
	dir := filepath.Join(base, domain, "resources")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte("tenant: "+domain+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestResolveExportDir covers the offline paths: an explicit domain never signs
// in, and without a credential the single export is the only accepted guess.
func TestResolveExportDir(t *testing.T) {
	ctx := context.Background()

	t.Run("declared domain runs offline", func(t *testing.T) {
		base := t.TempDir()
		dir, domain, err := ResolveExportDir(ctx, base, "contoso.example.com", nil)
		if err != nil || dir != filepath.Join(base, "contoso.example.com") || domain != "contoso.example.com" {
			t.Errorf("ResolveExportDir() = %q, %q, %v", dir, domain, err)
		}
	})

	t.Run("no credential falls back to the single export", func(t *testing.T) {
		base := t.TempDir()
		seedExport(t, base, "fabrikam.example.com")
		dir, _, err := ResolveExportDir(ctx, base, "", nil)
		if err != nil || dir != filepath.Join(base, "fabrikam.example.com") {
			t.Errorf("ResolveExportDir() = %q, %v; want the only export", dir, err)
		}
	})

	t.Run("unreadable output directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent")
		_, _, err := ResolveExportDir(ctx, missing, "", nil)
		if err == nil || !strings.Contains(err.Error(), "cannot read output directory") || !strings.Contains(err.Error(), "(pass --domain)") {
			t.Errorf("missing output directory: err = %v", err)
		}
	})

	t.Run("refuses to guess", func(t *testing.T) {
		empty := t.TempDir()
		if _, _, err := ResolveExportDir(ctx, empty, "", nil); err == nil || !strings.Contains(err.Error(), "no export directory") {
			t.Errorf("no exports: err = %v", err)
		}
		several := t.TempDir()
		seedExport(t, several, "a.example.com")
		seedExport(t, several, "b.example.com")
		if _, _, err := ResolveExportDir(ctx, several, "", nil); err == nil || !strings.Contains(err.Error(), "several export directories") {
			t.Errorf("several exports: err = %v", err)
		}
	})
}
