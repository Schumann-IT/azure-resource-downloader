package cmd

import "testing"

// TestDocsGroupFlagSurface guards the docs group's flag surface in both
// directions, like the resource group's test: every subcommand offers the
// shared export flags it honours (--domain and --out, declared per subcommand
// through one helper), and no subcommand offers a sibling's flag it would
// ignore. The credentials these commands may need to resolve the tenant domain
// come from the tenant's configuration profile, not from flags.
func TestDocsGroupFlagSurface(t *testing.T) {
	docsCmd := NewCommand()

	shared := []string{"domain", "out"}
	for _, name := range []string{"generate-prompt", "generate-index", "analyze-drift"} {
		sub := subcommand(t, docsCmd, name)
		for _, flag := range shared {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("docs %s is missing --%s", name, flag)
			}
		}
	}

	// --prompt overrides a template, which only the two prompt-emitting
	// commands have; --exit-code gates CI on stale documents, which only
	// generate-prompt decides.
	for name, flags := range map[string][]string{
		"generate-prompt": {"prompt", "exit-code"},
		"analyze-drift":   {"prompt"},
	} {
		sub := subcommand(t, docsCmd, name)
		for _, flag := range flags {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("docs %s is missing --%s", name, flag)
			}
		}
	}
	for name, flags := range map[string][]string{
		"generate-index": {"prompt", "exit-code"},
		"analyze-drift":  {"exit-code"},
		// analyze-consistency writes a fixed tree and has no prompt yet, so it
		// offers --domain alone.
		"analyze-consistency": {"out", "prompt", "exit-code"},
	} {
		sub := subcommand(t, docsCmd, name)
		for _, flag := range flags {
			if sub.Flags().Lookup(flag) != nil {
				t.Errorf("docs %s offers --%s but ignores it", name, flag)
			}
		}
	}
	if subcommand(t, docsCmd, "analyze-consistency").Flags().Lookup("domain") == nil {
		t.Error("docs analyze-consistency is missing --domain")
	}
}
