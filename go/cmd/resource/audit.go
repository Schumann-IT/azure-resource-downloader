package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"azure-resource-downloader/internal/audit"
	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/version"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// auditExitCannotAnswer marks a resource audit run that could answer nothing:
// no workspace configured, no current observation, the wrong tenant, or no
// audit table could be queried. A scripted rerun must notice.
const auditExitCannotAnswer = 2

// NewAuditCommand builds the `resource audit` command: who changed each
// drifted resource, and when? It re-attributes the latest drift observation
// from the tenant's Log Analytics audit tables, so a run made minutes after a
// change — before its audit record was ingested — can be completed later
// without fetching the tenant again.
func NewAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Attribute each drift finding to an actor and a time from the audit logs",
		Long: `Attribute every finding of the latest drift observation (drift/metadata.yaml)
to the actor and time recorded in the tenant's Log Analytics audit tables —
IntuneAuditLogs for Intune types, AuditLogs for Entra ID types — and write the
result to drift/audit.yaml beside the observation. Nothing else is written, and
nothing under resources/ or docs/ is touched.

The workspace is the tenant profile's 'audit-workspace-id' (a workspace GUID);
there is no flag for it. Prerequisites: diagnostic settings shipping both
tables to that workspace, and 'Log Analytics Reader' on it (a dedicated app
registration also needs the Log Analytics API delegated permission Data.Read).

The window queried is the observation's own: from the baseline's generatedAt to
the observation's observedAt. Every finding gets exactly one status: matched
(with its events, newest first), no-event-in-window, no-join-key (singletons and
ids that are no audit target), retention-exceeded (the window starts before the
table's earliest row), query-failed or not-queried (ARM types, types outside
this run's selection). Attribution is facts only; it never changes a verdict.

Audit records arrive with an ingestion lag, so an attribution made right after
'resource drift' may miss the latest change: run this command again later. When
the profile sets 'audit-workspace-id', 'resource drift' already attributes the
observation it writes; this command refreshes that file.

--type, --resource-id and --resource-group narrow the attributed findings;
findings outside the selection are recorded as not-queried. ARM findings are
never queried, so --resource-group yields a file of not-queried findings.
Under --dry-run the queries run and the report prints, but audit.yaml is not
written.

Exit codes: 0 when at least one audit table could be queried; 2 when nothing
could be answered (no audit-workspace-id, no drift observation, an observation
superseded by a newer download, a different tenant, or no table queryable).

Examples:
  # Attribute the latest drift observation
  azure-rd resource audit --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com

  # Only the Intune configuration policies
  azure-rd resource audit --type "Microsoft.Graph/deviceManagementConfigurationPolicies"

  # Query and report, leave audit.yaml untouched
  azure-rd resource audit --dry-run`,
		RunE: runAudit,
	}
	// The selection flags and --domain are inherited from the `resource` parent;
	// the workspace is configuration, never a flag.
	return cmd
}

func runAudit(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	log := logger.Default
	dryRun := viper.GetBool("dry-run")

	workspaceID := strings.TrimSpace(viper.GetString(config.AuditWorkspaceKey))
	if workspaceID == "" {
		return cmdutil.WithExitCode(auditExitCannotAnswer, fmt.Errorf(
			"no audit workspace configured: set %s to the Log Analytics workspace id in the tenant profile (<config-dir>/<domain>.yaml)",
			config.AuditWorkspaceKey))
	}

	// One credential for the whole run: it resolves the tenant and then mints
	// the Log Analytics token. No Graph probe, no dedicated-app prompt — this
	// command reads no Graph resource.
	cred, err := azure.NewCredential(viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		return cmdutil.WithExitCode(auditExitCannotAnswer, fmt.Errorf("cannot build a credential from the tenant profile: %w", err))
	}

	tenantDir, expectDomain, err := cmdutil.ResolveExportDir(ctx, viper.GetString("output"), cmdutil.DeclaredDomain(cmd), cred)
	if err != nil {
		return cmdutil.WithExitCode(auditExitCannotAnswer, fmt.Errorf("cannot resolve which export to attribute: %w", err))
	}

	obs, _, err := drift.CheckCurrent(tenantDir, expectDomain)
	if err != nil {
		return cmdutil.WithExitCode(auditExitCannotAnswer, auditPreflightError(err))
	}
	log.Info("Attributing drift observation", "dir", tenantDir, "observed_at", obs.ObservedAt, "findings", len(obs.Findings))

	sel := audit.Selection{
		Types:         viper.GetStringSlice("type"),
		ResourceIDs:   viper.GetStringSlice("resource-id"),
		ResourceGroup: viper.GetString("resource-group"),
	}
	if sel.ResourceGroup != "" {
		log.Warn("--resource-group selects ARM resources only, and ARM findings are never queried: every finding will be not-queried")
	}

	// The registry only routes findings to tables; its credential is lazy, so
	// building it makes no network call.
	registry := handlers.NewRegistry(cred, "", false)
	a := attributeObservation(ctx, newQuerier(cred), registry, obs, workspaceID, sel)

	path, err := persistAttribution(tenantDir, a, dryRun)
	if err != nil {
		return fmt.Errorf("failed to write the drift attribution: %w", err)
	}
	reportAttribution(a, path, dryRun)

	if noTableQueried(a) {
		return cmdutil.WithExitCode(auditExitCannotAnswer, errors.New("no audit table could be queried (see the table status above)"))
	}
	return nil
}

// auditPreflightError maps the currency check's refusals to actionable
// messages.
func auditPreflightError(err error) error {
	switch {
	case errors.Is(err, drift.ErrNoObservation):
		return fmt.Errorf("no drift observation to attribute; run 'azure-rd resource drift' first: %w", err)
	case errors.Is(err, drift.ErrObservationSuperseded):
		return fmt.Errorf("the drift observation was superseded by a newer download; run 'azure-rd resource drift' again: %w", err)
	case errors.Is(err, docs.ErrTenantMismatch):
		return fmt.Errorf("refusing to attribute another tenant's observation: %w", err)
	case errors.Is(err, docs.ErrNoMetadata):
		return fmt.Errorf("no export metadata found; run 'azure-rd resource download' first: %w", err)
	default:
		return fmt.Errorf("cannot read the drift observation: %w", err)
	}
}

// attributeObservation queries the audit tables for obs under a context
// bounded by the configured timeout, so a token request that would need an
// interactive sign-in for the Log Analytics audience cannot hold the run open;
// its failure degrades to query-failed.
func attributeObservation(ctx context.Context, q audit.Querier, registry *handlers.Registry,
	obs drift.Observation, workspaceID string, sel audit.Selection) *drift.Attribution {
	timeout := viper.GetInt("timeout")
	if timeout <= 0 {
		timeout = cmdutil.DefaultTimeoutSeconds
	}
	qctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	opts := audit.Options{WorkspaceID: workspaceID, ToolVersion: version.Tool(), Selection: sel}
	return audit.Attribute(qctx, q, registry, obs, opts)
}

// newQuerier builds the Log Analytics querier over cred; a client that cannot
// be built becomes a querier failing every query with that error, so the
// artifact still records every finding.
func newQuerier(cred azcore.TokenCredential) audit.Querier {
	q, err := audit.NewLogsQuerier(cred)
	if err != nil {
		return failingQuerier{err: err}
	}
	return q
}

// failingQuerier answers every query with the error that prevented building
// the real client, so the artifact still records every finding.
type failingQuerier struct{ err error }

// Query implements audit.Querier.
func (f failingQuerier) Query(context.Context, string, string, *time.Time, *time.Time) ([]audit.Row, error) {
	return nil, f.err
}

// persistAttribution writes audit.yaml, or under dryRun withholds it and says
// that an earlier file was not refreshed.
func persistAttribution(tenantDir string, a *drift.Attribution, dryRun bool) (string, error) {
	if dryRun {
		noteStaleFile(drift.AuditPath(tenantDir), "An attribution from an earlier run is still on disk and will NOT be refreshed by this dry run")
	}
	return drift.WriteAttribution(tenantDir, a, dryRun)
}

// noteStaleFile prints the path and age of an existing file so a dry run
// cannot be mistaken for having refreshed it.
func noteStaleFile(path, message string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	logger.Default.Warn(message, "path", path, "age", time.Since(info.ModTime()).Round(time.Second).String())
}

// noTableQueried reports whether every audit table failed.
func noTableQueried(a *drift.Attribution) bool {
	for _, ts := range a.Tables {
		if ts.Status == drift.TableStatusOK {
			return false
		}
	}
	return true
}

// reportAttribution prints the per-table status and the status counts.
func reportAttribution(a *drift.Attribution, path string, dryRun bool) {
	log := logger.Default
	tables := make([]string, 0, len(a.Tables))
	for name := range a.Tables {
		tables = append(tables, name)
	}
	sort.Strings(tables)
	for _, name := range tables {
		ts := a.Tables[name]
		if ts.Status == drift.TableStatusOK {
			log.Info("Audit table queried", "table", name, "earliest", ts.Earliest)
			continue
		}
		log.Warn("Audit table could not be queried; its findings are query-failed", "table", name, "reason", ts.Reason)
	}

	c := a.Counts
	log.Info("Attribution Summary",
		"matched", c.Matched,
		"no_event_in_window", c.NoEventInWindow,
		"no_join_key", c.NoJoinKey,
		"retention_exceeded", c.RetentionExceeded,
		"query_failed", c.QueryFailed,
		"not_queried", c.NotQueried)
	if c.NoEventInWindow > 0 {
		log.Info("Audit records arrive with an ingestion lag; rerun 'azure-rd resource audit' later to catch recent changes")
	}

	keys := make([]string, 0, len(a.Findings))
	for key := range a.Findings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		f := a.Findings[key]
		if f.Status == drift.AttributionMatched {
			latest := f.Events[0]
			log.Info("  changed by", "resource", key, "actor", latest.Actor, "at", latest.At, "activity", latest.Activity, "events", len(f.Events))
			continue
		}
		log.Debug("  "+f.Status, "resource", key, "reason", f.Reason)
	}

	if dryRun {
		log.Info("Dry-run: attribution not written", "would_write", path)
		return
	}
	log.Info("Drift attribution written", "path", path)
}
