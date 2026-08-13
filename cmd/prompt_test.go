package cmd

import (
	"strings"
	"testing"
)

func TestPromptForDedicatedApp(t *testing.T) {
	req := map[string][]string{
		"Microsoft.Graph/conditionalAccessPolicies": {"Policy.Read.All"},
	}

	t.Run("reads client and tenant id", func(t *testing.T) {
		in := strings.NewReader("app-id\ntenant-id\n")
		clientID, tenantID, err := promptForDedicatedApp(req, in, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clientID != "app-id" || tenantID != "tenant-id" {
			t.Errorf("got (%q, %q), want (app-id, tenant-id)", clientID, tenantID)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		in := strings.NewReader("  app-id  \n  tenant-id \n")
		clientID, tenantID, err := promptForDedicatedApp(req, in, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clientID != "app-id" || tenantID != "tenant-id" {
			t.Errorf("got (%q, %q), want (app-id, tenant-id)", clientID, tenantID)
		}
	})

	t.Run("blank input accepts defaults", func(t *testing.T) {
		in := strings.NewReader("\n\n")
		clientID, tenantID, err := promptForDedicatedApp(req, in, "env-client", "cli-tenant")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clientID != "env-client" || tenantID != "cli-tenant" {
			t.Errorf("got (%q, %q), want (env-client, cli-tenant)", clientID, tenantID)
		}
	})

	t.Run("explicit input overrides defaults", func(t *testing.T) {
		in := strings.NewReader("typed-client\n\n")
		clientID, tenantID, err := promptForDedicatedApp(req, in, "env-client", "cli-tenant")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clientID != "typed-client" || tenantID != "cli-tenant" {
			t.Errorf("got (%q, %q), want (typed-client, cli-tenant)", clientID, tenantID)
		}
	})

	t.Run("non-interactive with defaults succeeds", func(t *testing.T) {
		in := strings.NewReader("")
		clientID, tenantID, err := promptForDedicatedApp(req, in, "env-client", "cli-tenant")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if clientID != "env-client" || tenantID != "cli-tenant" {
			t.Errorf("got (%q, %q), want (env-client, cli-tenant)", clientID, tenantID)
		}
	})

	t.Run("errors on no input and no defaults", func(t *testing.T) {
		in := strings.NewReader("")
		if _, _, err := promptForDedicatedApp(req, in, "", ""); err == nil {
			t.Error("expected an error when no input and no defaults are available")
		}
	})

	t.Run("errors when tenant id is empty and no default", func(t *testing.T) {
		in := strings.NewReader("app-id\n\n")
		if _, _, err := promptForDedicatedApp(req, in, "", ""); err == nil {
			t.Error("expected an error when tenant id is empty")
		}
	})
}
