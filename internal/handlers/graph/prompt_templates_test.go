package graph

import (
	"strings"
	"testing"
)

// TestSharedPromptTemplateOverrides verifies that one representative handler
// per shared template category renders its category-specific prompt instead of
// the default settings/assignments layout.
func TestSharedPromptTemplateOverrides(t *testing.T) {
	tests := []struct {
		name        string
		newHandler  func() (*GraphCollectionHandler, error)
		marker      string
		description string
	}{
		{
			name:        "singleton",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewOrganizationHandler(fakeTokenCredential{}) },
			marker:      "tenant-wide singleton",
			description: "organization uses the singleton template",
		},
		{
			name:        "credential",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewVppTokenHandler(fakeTokenCredential{}) },
			marker:      "Expiry & renewal:",
			description: "vppTokens uses the credential template",
		},
		{
			name:        "record",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewDeviceCategoryHandler(fakeTokenCredential{}) },
			marker:      "inventory or registry record",
			description: "deviceCategories uses the record template",
		},
		{
			name:        "referenced",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewAssignmentFilterHandler(fakeTokenCredential{}) },
			marker:      "Usage & references:",
			description: "assignmentFilters uses the referenced template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := tt.newHandler()
			if err != nil {
				t.Fatalf("constructor unexpected error: %v", err)
			}

			prompt := handler.GetDocumentationPrompt()

			if !strings.Contains(prompt, tt.marker) {
				t.Errorf("%s: prompt missing %q", tt.description, tt.marker)
			}
			if !strings.Contains(prompt, "Azure resource type: "+handler.GetType()) {
				t.Errorf("%s: prompt missing resource type line", tt.description)
			}
			if strings.Contains(prompt, "a table of any assignments/targeting present") {
				t.Errorf("%s: prompt unexpectedly contains default-template assignments text", tt.description)
			}
		})
	}
}
