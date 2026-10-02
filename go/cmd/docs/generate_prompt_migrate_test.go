package docs

import (
	"reflect"
	"testing"

	docsengine "azure-resource-downloader/internal/docs"
)

func TestMigrateByDocument(t *testing.T) {
	items := []docsengine.WorkItem{
		{DocPath: "docs/b.md", Reason: "r1"},
		{DocPath: "docs/a.md", Reason: "only"},
		{DocPath: "docs/b.md", Reason: "r2"},
	}
	docs, reasons := migrateByDocument(items)
	if want := []string{"docs/a.md", "docs/b.md"}; !reflect.DeepEqual(docs, want) {
		t.Errorf("docs = %v, want %v", docs, want)
	}
	if reasons["docs/b.md"] != "r1; r2" || reasons["docs/a.md"] != "only" {
		t.Errorf("reasons = %v", reasons)
	}
}
