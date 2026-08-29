package arm

import (
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

// TestArmPromptTemplateOverride verifies that all ARM handlers render the
// shared ARM prompt template instead of the default settings/assignments
// layout, which does not apply to ARM resources.
func TestArmPromptTemplateOverride(t *testing.T) {
	handlers := []interface {
		GetType() string
		GetDocumentationPrompt() string
	}{
		NewResourceGroupHandler(nil, "00000000-0000-0000-0000-000000000000"),
		NewStorageAccountHandler(nil, "00000000-0000-0000-0000-000000000000"),
		NewVirtualMachineHandler(nil, "00000000-0000-0000-0000-000000000000"),
	}

	for _, handler := range handlers {
		t.Run(handler.GetType(), func(t *testing.T) {
			prompt := handler.GetDocumentationPrompt()

			for _, want := range []string{
				"senior Azure infrastructure consultant",
				"Azure resource type: " + handler.GetType(),
				"RBAC role assignments are NOT part of this export",
				"Properties:",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt missing %q", want)
				}
			}
			if strings.Contains(prompt, "a table of any assignments/targeting present") {
				t.Error("prompt unexpectedly contains default-template assignments text")
			}
			if !strings.Contains(models.DefaultDocumentationPromptTemplate(), "assignments/targeting") {
				t.Error("sanity check: default template should mention assignments/targeting")
			}
		})
	}
}
