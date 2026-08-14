package docs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"azure-resource-downloader/cmd/cmdutil"
	"azure-resource-downloader/internal/azure"
	docsengine "azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Exit codes for docs generate-prompt. 0 is success (whether or not work was
// found, unless --exit-code); a distinct code marks "could not answer" so CI can
// tell an unanswerable run from a clean one.
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

	// It needs nothing beyond the tenant domain, so the plain CLI credential is
	// enough — but reuse the shared auth group for --subscription/--client-id/
	// --tenant-id parity with the other commands.
	cmdutil.AddAzureAuthFlags(cmd)

	f := cmd.Flags()
	f.String("domain", "", "export tenant domain (folder name under --output); skips authentication and runs offline")
	f.String("out", "", "path to write the prompt to (default: <output>/<tenant>/docs/generate.md)")
	f.String("prompt", "", "path to a template file overriding the built-in DOC-GENERATION-TEMPLATE.md")
	f.Bool("exit-code", false, "exit non-zero (3) when stale documents were found, for CI gating")

	return cmd
}

func runGeneratePrompt(cmd *cobra.Command, _ []string) error {
	cmdutil.BindFlags(cmd)

	ctx := context.Background()
	log := logger.Default

	baseOutput := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	domain := viper.GetString("domain")
	outPath := viper.GetString("out")
	promptPath := viper.GetString("prompt")
	exitCode := viper.GetBool("exit-code")

	// Resolve the export directory and the domain to cross-check metadata against.
	tenantDir, expectDomain, err := resolveExportDir(ctx, baseOutput, domain,
		viper.GetString("subscription"), viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		log.Error("Cannot resolve which export to document", "error", err)
		os.Exit(exitCannotAnswer)
	}
	log.Info("Documenting export", "dir", tenantDir)

	// Load the template: the embedded default, or a --prompt override.
	template := docsengine.DefaultGeneratePromptTemplate()
	if promptPath != "" {
		template, err = os.ReadFile(promptPath)
		if err != nil {
			log.Error("Cannot read --prompt template", "path", promptPath, "error", err)
			os.Exit(exitCannotAnswer)
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
			log.Error("No export metadata found; run 'azure-rd download' first", "error", err)
		case errors.Is(err, docsengine.ErrTenantMismatch):
			log.Error("Refusing to document the wrong export", "error", err)
		default:
			log.Error("Failed to generate documentation prompt", "error", err)
		}
		os.Exit(exitCannotAnswer)
	}

	reportGeneratePrompt(res, dryRun)

	if exitCode && len(res.ToGenerate) > 0 {
		os.Exit(exitStaleFound)
	}
	return nil
}

// resolveExportDir decides which export directory to document and the domain to
// cross-check metadata.yaml against. With --domain it runs offline; otherwise it
// authenticates to resolve the tenant domain, falling back to a single export
// directory under baseOutput when auth or resolution fails.
func resolveExportDir(ctx context.Context, baseOutput, domain, sub, clientID, tenantID string) (tenantDir, expectDomain string, err error) {
	log := logger.Default

	if domain != "" {
		return filepath.Join(baseOutput, domain), domain, nil
	}

	if azureClient, aerr := azure.NewClient(ctx, sub, clientID, tenantID); aerr != nil {
		log.Warn("Authentication failed; falling back to a single export directory (pass --domain to run offline)",
			"reason", azure.ErrorSummary(aerr))
	} else if d, derr := azureClient.GetTenantDomain(ctx); derr != nil {
		log.Warn("Could not resolve tenant domain; falling back to a single export directory",
			"reason", azure.ErrorSummary(derr))
	} else {
		return filepath.Join(baseOutput, d), d, nil
	}

	d, derr := detectSingleExportDomain(baseOutput)
	if derr != nil {
		return "", "", derr
	}
	log.Info("Defaulting to the only export directory found", "domain", d)
	return filepath.Join(baseOutput, d), d, nil
}

// detectSingleExportDomain returns the single sub-directory of baseOutput that
// contains resources/metadata.yaml. It refuses to guess when zero or several
// candidates exist.
func detectSingleExportDomain(baseOutput string) (string, error) {
	entries, err := os.ReadDir(baseOutput)
	if err != nil {
		return "", fmt.Errorf("cannot read output directory %q: %w (pass --domain)", baseOutput, err)
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(baseOutput, e.Name(), models.ResourcesDirName, docsengine.MetadataFileName)
		if info, statErr := os.Stat(metaPath); statErr == nil && !info.IsDir() {
			candidates = append(candidates, e.Name())
		}
	}
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return "", fmt.Errorf("no export directory under %q (expected <domain>/resources/metadata.yaml); pass --domain", baseOutput)
	default:
		return "", fmt.Errorf("several export directories under %q (%v); pass --domain to choose", baseOutput, candidates)
	}
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

	if len(res.ToGenerate) == 0 {
		log.Info("Every in-scope document is current; nothing to generate")
		return
	}

	log.Info("Documents to generate", "count", len(res.ToGenerate))
	for _, it := range res.ToGenerate {
		log.Info("  needs documentation", "doc", it.DocPath, "reason", it.Reason)
	}

	if dryRun {
		log.Info("Dry-run: prompt not written", "would_write", res.OutPath)
		return
	}
	log.Info("Documentation prompt written", "path", res.OutPath, "documents", len(res.ToGenerate))
}
