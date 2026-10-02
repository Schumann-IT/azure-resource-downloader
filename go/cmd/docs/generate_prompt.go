package docs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"azure-resource-downloader/internal/cmdutil"
	docsengine "azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Exit codes for docs generate-prompt, carried to the root Execute via
// cmdutil.WithExitCode. 0 is success (whether or not work was found, unless
// --exit-code); a distinct code marks "could not answer" so CI can tell an
// unanswerable run from a clean one.
const (
	exitCannotAnswer = 2
	exitStaleFound   = 3
)

// NewGeneratePromptCommand builds the `docs generate-prompt` subcommand. It
// emits a ready-to-use documentation prompt covering exactly the resources
// whose documentation is missing or out of date. It is exported so the parent
// `docs` command (in package cmd) can attach it without an import cycle.
func NewGeneratePromptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-prompt",
		Short: "Emit a documentation prompt for resources whose docs are missing or stale",
		Long: `Compare an export's resources/metadata.yaml against the documents under
docs/ and write a single prompt file naming exactly the documents that need to
be generated.

This command never fetches a resource and never writes into resources/. It
authenticates only to resolve the tenant's Entra default domain (the export
folder name), exactly as 'download' does. Pass --domain to skip authentication
entirely and run offline against a named export folder.

The prompt is written to output/<tenant>/docs/generate.md (override with --out)
and overwritten on every run. Under --dry-run nothing is written; the command
lists the resources whose documentation needs refreshing.

Examples:
  # Resolve the tenant via 'az login', then write the prompt
  azure-rd docs generate-prompt

  # Offline: name the export folder explicitly (no sign-in)
  azure-rd docs generate-prompt --domain contoso.onmicrosoft.com

  # Preview what is stale without writing the prompt
  azure-rd docs generate-prompt --domain contoso.onmicrosoft.com --dry-run

  # Fail (exit 3) when stale documents exist, for CI gating
  azure-rd docs generate-prompt --domain contoso.onmicrosoft.com --exit-code`,
		RunE: runGeneratePrompt,
	}

	addExportFlags(cmd, "path to write the prompt to (default: <output>/<tenant>/docs/generate.md)")

	f := cmd.Flags()
	f.String("prompt", "", "path to a template file overriding the built-in documentation-prompt template")
	f.Bool("exit-code", false, "exit non-zero (3) when stale documents were found, for CI gating")

	return cmd
}

func runGeneratePrompt(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	log := logger.Default

	baseOutput := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	domain := cmdutil.DeclaredDomain(cmd)
	outPath, _ := cmd.Flags().GetString("out")
	promptPath, _ := cmd.Flags().GetString("prompt")
	exitCode, _ := cmd.Flags().GetBool("exit-code")

	// Resolve the export directory and the domain to cross-check metadata against.
	tenantDir, expectDomain, err := cmdutil.ResolveExportDir(ctx, baseOutput, domain, exportCredential(domain))
	if err != nil {
		return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("cannot resolve which export to document: %w", err))
	}
	log.Info("Documenting export", "dir", tenantDir)

	// Load the template: the embedded default, or a --prompt override.
	template := docsengine.DefaultGeneratePromptTemplate()
	if promptPath != "" {
		template, err = os.ReadFile(promptPath)
		if err != nil {
			return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("cannot read --prompt template: %w", err))
		}
	}

	// A stale generate.md from an earlier run outlives a dry run — surface it so
	// nobody pastes an out-of-date prompt.
	resolvedOut := outPath
	if resolvedOut == "" {
		resolvedOut = filepath.Join(tenantDir, docsengine.DocsDirName, docsengine.GenerateFileName)
	}
	if dryRun {
		noteStaleGeneratePrompt(resolvedOut)
	}

	res, err := docsengine.GeneratePrompt(docsengine.GeneratePromptOptions{
		TenantDir:    tenantDir,
		ExpectDomain: expectDomain,
		Template:     template,
		OutPath:      outPath,
		DryRun:       dryRun,
	})
	if err != nil {
		switch {
		case errors.Is(err, docsengine.ErrNoMetadata):
			err = fmt.Errorf("no export metadata found; run 'azure-rd resource download' first: %w", err)
		case errors.Is(err, docsengine.ErrTenantMismatch):
			err = fmt.Errorf("refusing to document the wrong export: %w", err)
		default:
			err = fmt.Errorf("failed to generate documentation prompt: %w", err)
		}
		return cmdutil.WithExitCode(exitCannotAnswer, err)
	}

	reportGeneratePrompt(res, dryRun)

	// --exit-code gates on ANY pending work — documents to generate, blocks to
	// re-splice, or documents to migrate — so a clean CI run means every
	// document on disk matches the export, not merely that none is missing.
	if exitCode && res.HasPendingWork() {
		return cmdutil.WithExitCode(exitStaleFound, errors.New("stale documentation found (see the report above)"))
	}
	return nil
}

// noteStaleGeneratePrompt prints the path and age of an existing generate.md so
// a dry run cannot be mistaken for having refreshed it.
func noteStaleGeneratePrompt(outPath string) {
	info, err := os.Stat(outPath)
	if err != nil {
		return
	}
	logger.Default.Warn("A prompt file from an earlier run is still on disk and will NOT be replaced by this dry run",
		"path", outPath,
		"age", time.Since(info.ModTime()).Round(time.Second).String())
}

// reportGeneratePrompt prints the outcome, including export freshness so a
// forgotten download step is visible.
func reportGeneratePrompt(res *docsengine.GeneratePromptResult, dryRun bool) {
	log := logger.Default

	complete := "yes"
	if !res.ExportComplete {
		complete = fmt.Sprintf("no (%s)", res.IncompleteReason)
	}
	log.Info("Export status",
		"generated_at", res.ExportGeneratedAt,
		"complete", complete,
		"referenced_groups", res.ReferencedGroups)
	if !res.ExportComplete {
		log.Warn("The export is marked incomplete; it may lag the tenant. Documenting what is present is still useful")
	}

	for _, t := range res.PromptMissingTypes {
		log.Warn("Type has no doc-prompt.md; its documents cannot be generated (was the export run with --no-prompt?)", "type", t)
	}
	for _, o := range res.Orphans {
		log.Warn("Orphaned document: resource is no longer in the tenant (left in place, not deleted)", "source", o)
	}
	for _, id := range res.DanglingGroupIDs {
		log.Warn("Dangling assignment target: group not in export", "group_id", id)
	}
	for _, id := range res.DanglingFilterIDs {
		log.Warn("Dangling assignment target: filter not in export", "filter_id", id)
	}
	for _, id := range res.DanglingTemplateIDs {
		log.Warn("Dangling notification template reference: template not in export", "template_id", id)
	}

	// The three work sets are independent: a run can have nothing to generate
	// yet still need blocks re-spliced or documents migrated.
	if !res.HasPendingWork() {
		log.Info("Every in-scope document is current; nothing to generate, re-splice or migrate")
	}

	if len(res.ToGenerate) > 0 {
		log.Info("Documents to generate", "count", len(res.ToGenerate))
		for _, it := range res.ToGenerate {
			log.Info("  needs documentation", "doc", it.DocPath, "reason", it.Reason)
		}
	}
	if len(res.Migrate) > 0 {
		docs, reasons := migrateByDocument(res.Migrate)
		log.Info("Documents to migrate to assignment or noncompliance-notification markers", "count", len(docs))
		for _, doc := range docs {
			log.Info("  needs marker migration", "doc", doc, "reason", reasons[doc])
		}
	}
	if len(res.ForwardResplice) > 0 {
		log.Info("Documents whose assignments block must be re-spliced", "count", len(res.ForwardResplice))
		for _, it := range res.ForwardResplice {
			log.Info("  assignments block stale", "doc", it.DocPath, "reason", it.Reason)
		}
	}
	if len(res.ReverseResplice) > 0 {
		log.Info("Group documents whose 'Targeted by' block must be re-spliced", "count", len(res.ReverseResplice))
		for _, it := range res.ReverseResplice {
			log.Info("  targeting block stale", "doc", it.DocPath, "reason", it.Reason)
		}
	}
	if len(res.UsedByResplice) > 0 {
		log.Info("Notification template documents whose 'Used by' block must be re-spliced", "count", len(res.UsedByResplice))
		for _, it := range res.UsedByResplice {
			log.Info("  usage block stale", "doc", it.DocPath, "reason", it.Reason)
		}
	}
	if len(res.NotificationsResplice) > 0 {
		log.Info("Policy documents whose noncompliance-notification block must be re-spliced", "count", len(res.NotificationsResplice))
		for _, it := range res.NotificationsResplice {
			log.Info("  notification block stale", "doc", it.DocPath, "reason", it.Reason)
		}
	}

	if dryRun {
		log.Info("Dry-run: prompt not written", "would_write", res.OutPath)
		return
	}
	if res.HasPendingWork() {
		log.Info("Documentation prompt written", "path", res.OutPath,
			"generate", len(res.ToGenerate),
			"migrate", len(res.Migrate),
			"resplice_forward", len(res.ForwardResplice),
			"resplice_reverse", len(res.ReverseResplice),
			"resplice_used_by", len(res.UsedByResplice),
			"resplice_notifications", len(res.NotificationsResplice))
	}
}

// migrateByDocument groups migrate items by document: a document missing both
// the assignment and the notification markers has two items but is one
// document to migrate. It returns the distinct document paths in sorted order
// and each document's item reasons joined with "; " in item order.
func migrateByDocument(items []docsengine.WorkItem) ([]string, map[string]string) {
	reasons := map[string]string{}
	var docs []string
	for _, it := range items {
		prev, seen := reasons[it.DocPath]
		if !seen {
			docs = append(docs, it.DocPath)
		}
		reasons[it.DocPath] = strings.Join(nonEmpty(prev, it.Reason), "; ")
	}
	sort.Strings(docs)
	return docs, reasons
}

// nonEmpty returns the arguments that are not blank, in order.
func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
