package docs

import (
	"errors"
	"fmt"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/consistency"
	docsengine "azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/version"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewAnalyzeConsistencyCommand builds the `docs analyze-consistency`
// subcommand. It indexes every configured setting of an export, models every
// resource's assignment scope and writes the same-setting conflicts and
// duplicates it finds to the consistency/ tree. It is exported so the parent
// `docs` command (in package cmd) can attach it without an import cycle.
func NewAnalyzeConsistencyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze-consistency",
		Short: "Report settings two resources configure differently or redundantly for overlapping scopes",
		Long: `Index every configured setting of an export — Settings Catalog and compliance
policies, custom OMA-URI and typed device configurations, typed compliance
policies, Administrative Templates and intents — model every resource's
assignment scope (include and exclude groups, All devices / All users, filters,
platform), and report where two resources whose scopes can reach the same
device or user configure the same setting:

  conflict       two enforcing sides with different values
  duplicate      the same value, set twice
  contradiction  a configuration value that fails a compliance requirement
                 (only through an equivalence joining the two keys)

Each finding carries its overlap verdict (certain or possible). A pair whose
value is unknown — a secret, or a value that cannot be compared across policy
types — is listed without values, never compared. Secrets never leave the
export, even when 'resolve-secrets' wrote them in plaintext.

This command never fetches a resource and reads only resources/ (the YAML and
metadata.yaml), so it can run right after 'resource download'. It authenticates
only to resolve the tenant's Entra default domain, exactly as 'download' does.
Pass --domain to skip authentication entirely and run offline.

It writes output/<tenant>/consistency/mechanical.yaml (the findings) and
consistency/metadata.yaml (the export it describes and the counts), and
leaves other files there alone. A re-baselining 'resource download' clears the
consistency/ tree. Findings never change the exit code; it exits 2 when there
is no export, the export belongs to another tenant, or a write fails.

With a 'consistency:' section in the base configuration file (the catalog:
equivalences, topics and rules), the equivalences join keys across policy
types — compliance against configuration included — and consistency/
metadata.yaml records a catalog: block (its hash, its counts and the
equivalence members this export never sets). An invalid section exits 2
before the export is resolved or anyone signs in.

Under --dry-run nothing is written and nothing is cleared; the counts are
reported.

Examples:
  # Resolve the tenant via 'az login', then analyze
  azure-rd docs analyze-consistency

  # Offline: name the export folder explicitly (no sign-in)
  azure-rd docs analyze-consistency --domain contoso.onmicrosoft.com

  # Report the counts without writing the tree
  azure-rd docs analyze-consistency --domain contoso.onmicrosoft.com --dry-run`,
		RunE: runAnalyzeConsistency,
	}

	// The tree is fixed, so there is no --out; --prompt waits for the analysis
	// prompt.
	addDomainFlag(cmd)

	return cmd
}

func runAnalyzeConsistency(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	log := logger.Default

	baseOutput := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	domain := cmdutil.DeclaredDomain(cmd)

	// The catalog is compiled before the export is resolved, so a typo in it
	// fails offline and before any sign-in.
	catalog, err := loadConsistencyCatalog()
	if err != nil {
		return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("invalid 'consistency' config section: %w", err))
	}

	tenantDir, expectDomain, err := cmdutil.ResolveExportDir(ctx, baseOutput, domain, exportCredential(domain))
	if err != nil {
		return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("cannot resolve which export to analyze: %w", err))
	}
	log.Info("Analyzing export consistency", "dir", tenantDir)

	res, err := consistency.Analyze(consistency.Options{
		TenantDir:    tenantDir,
		ExpectDomain: expectDomain,
		ToolVersion:  version.Tool(),
		Catalog:      catalog,
		DryRun:       dryRun,
	})
	if err != nil {
		switch {
		case errors.Is(err, docsengine.ErrNoMetadata):
			err = fmt.Errorf("no export metadata found; run 'azure-rd resource download' first: %w", err)
		case errors.Is(err, docsengine.ErrTenantMismatch):
			err = fmt.Errorf("refusing to analyze the wrong export: %w", err)
		default:
			err = fmt.Errorf("failed to analyze consistency: %w", err)
		}
		return cmdutil.WithExitCode(exitCannotAnswer, err)
	}

	reportAnalyzeConsistency(res, catalog, dryRun)
	return nil
}

// loadConsistencyCatalog reads and compiles the `consistency:` section of the
// configuration; nil when the section is absent.
func loadConsistencyCatalog() (*consistency.Compiled, error) {
	if !viper.IsSet("consistency") {
		return nil, nil
	}
	var cfg consistency.CatalogConfig
	if err := viper.UnmarshalKey("consistency", &cfg, func(c *mapstructure.DecoderConfig) {
		c.ErrorUnused = true // a misspelt optional field is refused, not silently ignored
	}); err != nil {
		return nil, err
	}
	return consistency.CompileCatalog(cfg)
}

// reportAnalyzeConsistency prints the counts and ends with one summary line per
// finding kind, and one catalog line when a catalog is configured.
func reportAnalyzeConsistency(res *consistency.Result, catalog *consistency.Compiled, dryRun bool) {
	log := logger.Default
	c := res.Metadata.Counts

	resources, settings := 0, 0
	for _, t := range sortedSourceTypes(c.Indexed) {
		sc := c.Indexed[t]
		resources += sc.Resources
		settings += sc.Settings
		if sc.Resources > 0 {
			log.Info("  indexed", "type", t, "resources", sc.Resources, "settings", sc.Settings)
		}
	}
	log.Info("Consistency summary",
		"tenant", res.Metadata.Tenant,
		"export_generated_at", res.Metadata.ExportGeneratedAt,
		"export_complete", res.Metadata.ExportComplete,
		"resources", resources,
		"settings", settings,
		"unreadable", c.Unreadable,
		"unknown_values", c.UnknownValues,
		"ruled_out_by_scope", c.RuledOutByScope)
	if c.Unreadable > 0 {
		log.Warn("Some resources could not be read and were not indexed", "unreadable", c.Unreadable)
	}

	if cat := res.Metadata.Catalog; cat != nil && catalog != nil {
		log.Info("Consistency catalog",
			"equivalences", cat.Counts.Equivalences,
			"topics", cat.Counts.Topics,
			"rules", cat.Counts.Rules,
			"unmatched_members", len(cat.UnmatchedMembers),
			"verify", catalog.VerifyCount())
	}

	if dryRun {
		log.Info("Dry-run: consistency tree not written", "would_write", res.Dir)
	} else {
		log.Info("Consistency analysis written", "dir", res.Dir)
	}
	for _, kind := range []string{consistency.KindConflict, consistency.KindContradiction, consistency.KindDuplicate} {
		byOverlap := c.Findings[kind]
		log.Info("  "+kind,
			"certain", byOverlap[consistency.OverlapCertain],
			"possible", byOverlap[consistency.OverlapPossible])
	}
}

// sortedSourceTypes returns the indexed source types in a stable order.
func sortedSourceTypes(m map[string]consistency.SourceCount) []string {
	counts := make(map[string]int, len(m))
	for k := range m {
		counts[k] = 0
	}
	return sortedCountKeys(counts)
}
