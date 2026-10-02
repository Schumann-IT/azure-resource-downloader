package graph

import (
	"strings"
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestDeviceComplianceScriptHandler_GetType(t *testing.T) {
	handler, err := NewDeviceComplianceScriptHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceComplianceScriptHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/deviceComplianceScripts"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestDeviceComplianceScriptHandler_Transform(t *testing.T) {
	handler, err := NewDeviceComplianceScriptHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceComplianceScriptHandler() unexpected error: %v", err)
	}

	script := betamodels.NewDeviceComplianceScript()
	id := "11111111-1111-1111-1111-111111111111"
	name := "Custom Compliance Script"
	script.SetId(&id)
	script.SetDisplayName(&name)

	transformed, err := handler.Transform(script)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != name {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, name)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/deviceComplianceScripts" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/deviceComplianceScripts")
	}
}

// TestDeviceComplianceScriptHandler_ReferencedFamily verifies the type has no
// assignments concept and documents with the referenced template's heading set:
// a script is attached to a compliance policy, not assigned itself.
func TestDeviceComplianceScriptHandler_ReferencedFamily(t *testing.T) {
	handler, err := NewDeviceComplianceScriptHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceComplianceScriptHandler() unexpected error: %v", err)
	}
	if handler.HasAssignments() {
		t.Error("HasAssignments() = true, want false")
	}
	const headings = "\n<!-- doc-headings: References | Usage and references | Lifecycle and operations | Security | Definition -->\n"
	if prompt := handler.GetDocumentationPrompt(); !strings.Contains(prompt, headings) {
		t.Errorf("prompt missing the referenced heading set %q", strings.TrimSpace(headings))
	}
}
