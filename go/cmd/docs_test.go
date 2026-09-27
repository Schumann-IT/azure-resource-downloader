package cmd

import "testing"

// TestDocsGroupFlagSurface guards the docs group's flag surface in both
// directions, like the resource group's test: every subcommand offers the
// shared export flags it honours (--domain/--out plus the auth group, declared
// per subcommand through one helper), and no subcommand offers a sibling's flag
// it would ignore.
func TestDocsGroupFlagSurface(t *testing.T) {
	docsCmd := NewCommand()

	shared := []string{"domain", "out", "subscription", "client-id", "tenant-id"}
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
	} {
		sub := subcommand(t, docsCmd, name)
		for _, flag := range flags {
			if sub.Flags().Lookup(flag) != nil {
				t.Errorf("docs %s offers --%s but ignores it", name, flag)
			}
		}
	}
}
