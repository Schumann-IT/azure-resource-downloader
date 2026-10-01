package pipeline

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"azure-resource-downloader/internal/handlers"
)

// The documentation-prompt golden test proves that a change to the prompt
// templates or to a handler's documentation metadata moves exactly the
// promptSha256 values it means to. For every registered type it compares the
// assembled doc-prompt.md — the bytes the writer puts on disk and hashes into
// promptSha256 — with a checked-in golden file. The files hold the full text,
// not a hash, so a deliberate template change is reviewable as a diff.
//
// Regenerating them is a deliberate step: set UPDATE_GOLDEN=1
// (`make golden-update`), which deletes and rewrites the prompts tree, and
// review the diff. A missing golden file, or a golden file for a type that is
// no longer registered, fails the test.

// goldenPromptDir holds one <resource type>/doc-prompt.md per registered type;
// the type path mirrors the export tree.
var goldenPromptDir = filepath.Join(goldenDir, "prompts")

// goldenPromptFile is the file name of each golden documentation prompt.
const goldenPromptFile = "doc-prompt.md"

func TestGoldenDocPrompts(t *testing.T) {
	registry := handlers.NewRegistry(offlineCredential{}, "00000000-0000-0000-0000-000000000000", false)
	types := registry.GetAllTypes()
	sort.Strings(types)

	if os.Getenv(updateGoldenEnv) != "" {
		rewriteGoldenPrompts(t, registry, types)
		return
	}

	registered := make(map[string]bool, len(types))
	for _, resourceType := range types {
		registered[resourceType] = true
		t.Run(resourceType, func(t *testing.T) {
			got := []byte(goldenPromptFor(t, registry, resourceType))
			goldenPath := goldenPromptPath(resourceType)
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("type %s: read golden prompt (create it deliberately with %s=1 / make golden-update): %v",
					resourceType, updateGoldenEnv, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("type %s: documentation prompt differs from %s\n%s", resourceType, goldenPath, firstDifference(want, got))
			}
		})
	}

	for _, resourceType := range goldenPromptTypes(t) {
		if !registered[resourceType] {
			t.Errorf("golden prompt %s belongs to no registered type (rewrite with %s=1 / make golden-update)",
				goldenPromptPath(resourceType), updateGoldenEnv)
		}
	}
}

// goldenPromptFor returns the exact doc-prompt.md content the writer would
// write and hash for resourceType.
func goldenPromptFor(t *testing.T, registry *handlers.Registry, resourceType string) string {
	t.Helper()
	h, err := registry.Get(resourceType)
	if err != nil {
		t.Fatalf("type %s: %v", resourceType, err)
	}
	return assembleDocPrompt(resourceType, h.GetDocumentationPrompt())
}

// goldenPromptPath maps a resource type to its golden file.
func goldenPromptPath(resourceType string) string {
	return filepath.Join(goldenPromptDir, filepath.FromSlash(resourceType), goldenPromptFile)
}

// rewriteGoldenPrompts deletes the prompts tree and writes one golden file per
// registered type, so a removed type leaves no stale file behind.
func rewriteGoldenPrompts(t *testing.T, registry *handlers.Registry, types []string) {
	t.Helper()
	if err := os.RemoveAll(goldenPromptDir); err != nil {
		t.Fatalf("remove golden prompts: %v", err)
	}
	for _, resourceType := range types {
		path := goldenPromptPath(resourceType)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("type %s: create golden prompt directory: %v", resourceType, err)
		}
		if err := os.WriteFile(path, []byte(goldenPromptFor(t, registry, resourceType)), 0o600); err != nil {
			t.Fatalf("type %s: write golden prompt: %v", resourceType, err)
		}
	}
}

// goldenPromptTypes lists the resource types that have a golden file on disk.
func goldenPromptTypes(t *testing.T) []string {
	t.Helper()
	var types []string
	err := filepath.WalkDir(goldenPromptDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(goldenPromptDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, "/"+goldenPromptFile) {
			t.Errorf("unexpected file in golden prompts tree: %s", path)
			return nil
		}
		types = append(types, strings.TrimSuffix(rel, "/"+goldenPromptFile))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk golden prompts: %v", err)
	}
	return types
}
