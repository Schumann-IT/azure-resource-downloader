package docs

import (
	"context"
	"errors"
	"os"
	"sort"

	"azure-resource-downloader/internal/cmdutil"
	docsengine "azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewGenerateIndexCommand builds the `docs generate-index` subcommand. It emits
// docs/index.yaml — the machine-readable navigation index the documentation
// frontend builds a tenant's index from. It is exported so the parent `docs`
// command (in package cmd) can attach it without an import cycle.
func NewGenerateIndexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-index",
		Short: "Emit docs/index.yaml, the navigation index for the documentation frontend",
		Long: `Build docs/index.yaml from an export's resources/metadata.yaml, enriched
with each document's frontmatter (its summary and platform/function grouping).
The documentation frontend reads this single file to render a tenant's index,
so no index.md is generated.

This command never fetches a resource and never writes into resources/. It
authenticates only to resolve the tenant's Entra default domain (the export
folder name), exactly as 'download' does. Pass --domain to skip authentication
entirely and run offline against a named export folder.

The index is written to output/<tenant>/docs/index.yaml (override with --out)
and overwritten on every run. Run it after a documentation pass so the newly
written documents' summaries and grouping are picked up; run before any
documents exist and every in-scope resource is listed as pending.

Under --dry-run nothing is written; the command reports what the index would
contain.

Examples:
  # Resolve the tenant via 'az login', then write the index
  azure-rd docs generate-index

  # Offline: name the export folder explicitly (no sign-in)
  azure-rd docs generate-index --domain contoso.onmicrosoft.com

  # Preview what the index would contain without writing it
  azure-rd docs generate-index --domain contoso.onmicrosoft.com --dry-run

  # Group resources into programmes: define a 'taxonomy:' section in a config
  # file (see config.example.yaml) and load it
  azure-rd docs generate-index --config azure-rd.yaml`,
		RunE: runGenerateIndex,
	}

	// It needs nothing beyond the tenant domain, so the plain CLI credential is
	// enough — but reuse the shared auth group for --subscription/--client-id/
	// --tenant-id parity with the other commands.
	cmdutil.AddAzureAuthFlags(cmd)

	f := cmd.Flags()
	f.String("domain", "", "export tenant domain (folder name under --output); skips authentication and runs offline")
	f.String("out", "", "path to write the index to (default: <output>/<tenant>/docs/index.yaml)")

	return cmd
}

func runGenerateIndex(cmd *cobra.Command, _ []string) error {
	cmdutil.BindFlags(cmd)

	ctx := context.Background()
	log := logger.Default

	baseOutput := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	domain := viper.GetString("domain")
	outPath := viper.GetString("out")

	// Grouping is driven entirely by the config file's `taxonomy:` section; read
	// it with UnmarshalKey so mapstructure's case-insensitive matching survives
	// viper's lowercasing of config keys (e.g. odataType -> odatatype).
	var taxonomy *docsengine.TaxonomyConfig
	if viper.IsSet("taxonomy") {
		var cfg docsengine.TaxonomyConfig
		if err := viper.UnmarshalKey("taxonomy", &cfg); err != nil {
			log.Error("Invalid 'taxonomy' config section", "error", err)
			os.Exit(exitCannotAnswer)
		}
		taxonomy = &cfg
	}

	// Resolve the export directory and the domain to cross-check metadata against.
	tenantDir, expectDomain, err := resolveExportDir(ctx, baseOutput, domain,
		viper.GetString("subscription"), viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		log.Error("Cannot resolve which export to index", "error", err)
		os.Exit(exitCannotAnswer)
	}
	log.Info("Indexing export", "dir", tenantDir)

	res, err := docsengine.GenerateIndex(docsengine.GenerateIndexOptions{
		TenantDir:    tenantDir,
		ExpectDomain: expectDomain,
		OutPath:      outPath,
		Taxonomy:     taxonomy,
		DryRun:       dryRun,
	})
	if err != nil {
		switch {
		case errors.Is(err, docsengine.ErrNoMetadata):
			log.Error("No export metadata found; run 'azure-rd download' first", "error", err)
		case errors.Is(err, docsengine.ErrTenantMismatch):
			log.Error("Refusing to index the wrong export", "error", err)
		default:
			log.Error("Failed to generate documentation index", "error", err)
		}
		os.Exit(exitCannotAnswer)
	}

	reportGenerateIndex(res, dryRun)
	return nil
}

// reportGenerateIndex prints the outcome, including export freshness and the
// pending/excluded breakdown so a forgotten documentation pass is visible.
func reportGenerateIndex(res *docsengine.GenerateIndexResult, dryRun bool) {
	log := logger.Default

	complete := "yes"
	if !res.Complete {
		complete = "no"
	}
	log.Info("Index summary",
		"tenant", res.Tenant,
		"generated_at", res.GeneratedAt,
		"complete", complete,
		"documented", res.Documented,
		"pending", res.Pending,
		"orphans", res.Orphans,
		"uncategorised", res.Uncategorised)

	if res.Uncategorised > 0 {
		log.Warn("Some resources matched no programme in the taxonomy; listed as uncategorised",
			"uncategorised", res.Uncategorised)
	}

	for _, t := range sortedExcludedTypes(res.Excluded) {
		log.Info("  excluded (not documented)", "type", t, "count", res.Excluded[t])
	}
	if res.Pending > 0 {
		log.Warn("Some in-scope resources have no document yet; listed as pending (run a documentation pass, then re-index)",
			"pending", res.Pending)
	}

	if dryRun {
		log.Info("Dry-run: index not written", "would_write", res.OutPath)
		return
	}
	log.Info("Documentation index written", "path", res.OutPath, "resources", res.Documented+res.Pending)
}

// sortedExcludedTypes returns the excluded-type keys in a stable order so the
// report does not depend on map iteration order.
func sortedExcludedTypes(excluded map[string]int) []string {
	types := make([]string, 0, len(excluded))
	for t := range excluded {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
