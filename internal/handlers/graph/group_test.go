package graph

import (
	"strings"
	"testing"
)

func TestGroupHandler_GetType(t *testing.T) {
	handler, err := NewGroupHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewGroupHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/groups"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestGroupHandler_GetDocumentationPromptUsesOverrideTemplate(t *testing.T) {
	handler, err := NewGroupHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewGroupHandler() unexpected error: %v", err)
	}

	prompt := handler.GetDocumentationPrompt()

	for _, want := range []string{
		"senior Microsoft cloud and identity consultant",
		"Azure resource type: Microsoft.Graph/groups",
		"Permissions required to read this resource type:\n- Group.Read.All",
		"Membership:",
		"Usage as assignment target:",
		"give particular attention to: groupTypes, membershipRule (for dynamic groups), securityEnabled, mailEnabled.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// The default template's settings/assignments layout must not leak in.
	for _, unwanted := range []string{
		"Then the following H2 sections, unnumbered, in this order:\n\nReferences:",
		"a table of any assignments/targeting present",
	} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("prompt unexpectedly contains default-template text %q", unwanted)
		}
	}
}
