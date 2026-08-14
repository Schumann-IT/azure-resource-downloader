package docs

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEmbeddedTemplateMatchesRoot guards against the package-local copy drifting
// from the repo-root DOC-GENERATION-TEMPLATE.md, which remains the source of
// truth. If this fails after editing the root template, re-copy it:
//
//	cp DOC-GENERATION-TEMPLATE.md internal/docs/generate_prompt_template.md
func TestEmbeddedTemplateMatchesRoot(t *testing.T) {
	rootPath := filepath.Join("..", "..", "DOC-GENERATION-TEMPLATE.md")
	root, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("read root template: %v", err)
	}
	if string(root) != string(generatePromptTemplate) {
		t.Errorf("embedded template has drifted from %s; re-copy it (cp DOC-GENERATION-TEMPLATE.md internal/docs/generate_prompt_template.md)", rootPath)
	}
}
