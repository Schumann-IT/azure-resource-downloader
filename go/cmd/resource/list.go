package resource

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewListCommand builds the `resource list` command: what does the tenant
// actually contain, per resource, without downloading anything. It shares the
// authentication, selection and --workers flags declared on the `resource`
// parent, and enumerates through the same listing path a download uses to build
// its fetch requests, so the two can never disagree about what is in scope.
func NewListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the resources the tenant contains",
		Long: `List the tenant's resources for the selected types, without downloading
anything. With no selection it covers every registered type, exactly as a full
download would; the enumeration is the same listing a download performs to build
its fetch requests, only rendered as "what is there" instead of "what would be
written".

Listing yields resource ids. When an export for the tenant already exists under
the output directory, display names recorded in its resources/metadata.yaml are
joined in and resources not present in the export are marked as new — nothing is
ever fetched merely to prettify the listing. A type that could not be listed
(e.g. missing permissions) is reported as unknown, never as empty, and does not
fail the command. To see what this build supports instead, use
'azure-rd resource types'.

This command writes nothing, so --dry-run changes nothing.

Examples:
  # Everything the tenant contains, across all registered types
  azure-rd resource list

  # One type only
  azure-rd resource list --type "Microsoft.Graph/groups"

  # With a dedicated app registration for Graph/Intune scopes
  azure-rd resource list --client-id "<app-id>" --tenant-id "<tenant-id>"`,
		RunE: runList,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	// Bind the flags that apply to this command (inherited from the resource
	// group and root) to viper before reading any values so the
	// flag > env > config > default precedence holds without a sibling command
	// stealing the binding.
	cmdutil.BindFlags(cmd)

	ctx := cmd.Context()
	log := logger.Default

	sub := viper.GetString("subscription")
	output := viper.GetString("output")
	clientID := viper.GetString("client-id")
	tenantID := viper.GetString("tenant-id")
	resourceIDs := viper.GetStringSlice("resource-id")
	selectedTypes := viper.GetStringSlice("type")
	resourceGroup := viper.GetString("resource-group")
	workersFlag := viper.GetInt("workers")
	workersExplicit := cmd.Flags().Changed("workers")

	// This command has no output artifact, so there is nothing for --dry-run to
	// withhold; say so instead of silently ignoring the flag.
	if viper.GetBool("dry-run") {
		log.Info("--dry-run has no effect here: this command writes nothing either way")
	}

	// Unlike `resource types`, nothing about this command's question is
	// answerable offline, so a missing session is a real error and is surfaced
	// up front — the same fail-fast the download performs. The device-code path
	// (--client-id/--tenant-id) is exempt: its sign-in happens at the first
	// token request.
	if clientID == "" {
		probeCred, err := azure.NewCredential("", "")
		if err != nil {
			return fmt.Errorf("failed to prepare Azure credentials: %w", err)
		}
		if err := azure.VerifySession(ctx, probeCred); err != nil {
			log.Debug("Session verification failed", "error", err)
			return fmt.Errorf("not signed in to Azure; run 'az login' first or pass --client-id/--tenant-id for device-code sign-in (%s)",
				azure.ErrorSummary(err))
		}
	}

	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub, clientID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}
	sub = azureClient.GetSubscriptionID()

	// Secret resolution is a download-only concern, so it is always disabled here.
	registry := handlers.NewRegistry(azureClient.GetCredential(), sub, false)

	// Enumerate through the exact listing path a download uses to build its
	// fetch requests: scope, filters and the treatment of unlistable types are
	// shared code, not a parallel implementation.
	workerConfig := BuildWorkerConfig(workersExplicit)
	requests, skippedTypes, emptyTypes, err := registry.BuildFetchRequests(ctx, resourceIDs, resourceGroup, selectedTypes, sub,
		listingConcurrency(workerConfig, workersFlag, workersExplicit))
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	// Display names come from a fetch and a transform, so a listing alone
	// cannot show them. Joining them from an existing export's recorded facts
	// is legitimate; fetching merely to prettify a listing is not.
	names, exportFound := exportNames(ctx, azureClient, output)

	// Group per type, deterministically ordered.
	byType := map[string][]string{}
	for _, r := range requests {
		key := r.ResourceType
		if key == "" {
			// Explicit --resource-id requests may carry no parseable type.
			key = "(from --resource-id)"
		}
		byType[key] = append(byType[key], r.ResourceID)
	}
	typeOrder := make([]string, 0, len(byType))
	for t := range byType {
		typeOrder = append(typeOrder, t)
	}
	sort.Strings(typeOrder)

	for _, t := range typeOrder {
		ids := byType[t]
		log.Info(t, "count", len(ids))
		for _, id := range ids {
			kv := []interface{}{"id", id}
			if exportFound {
				if name, ok := names[strings.ToLower(id)]; ok {
					kv = append(kv, "name", name)
				} else {
					// In the export's terms this resource is new: the tenant
					// has it, the export does not (yet).
					kv = append(kv, "new", true)
				}
			}
			log.Info("", kv...)
		}
	}

	// A type whose listing was refused has an unknown resource count — distinct
	// from a type that listed to zero — and never fails the run: missing
	// permissions warn, they do not fail.
	for _, st := range skippedTypes {
		log.Warn("Type could not be listed; its contents are unknown (not zero)", "type", st.ResourceType, "reason", st.Reason)
	}
	for _, t := range emptyTypes {
		log.Info("Type listed to zero resources", "type", t)
	}
	if !exportFound {
		log.Info("No export found for this tenant; display names are not shown (a download records them in resources/metadata.yaml)")
	}

	log.Info("Tenant listing finished",
		"resources", len(requests),
		"types_unknown", len(skippedTypes),
		"types_empty", len(emptyTypes))
	return nil
}

// exportNames joins the listing against an existing export for the tenant: it
// resolves the tenant domain, reads <output>/<tenant>/resources/metadata.yaml
// and returns recorded display names keyed by lowercased resource id. Every
// failure degrades to "no names" — the ids are the listing's substance, the
// names are recorded convenience.
func exportNames(ctx context.Context, azureClient *azure.Client, baseOutput string) (map[string]string, bool) {
	log := logger.Default

	tenantDomain, err := azureClient.GetTenantDomain(ctx)
	if err != nil {
		log.Debug("Tenant domain resolution failed", "error", err)
		log.Warn("Could not resolve the tenant domain; display names from an existing export are not shown",
			"reason", azure.ErrorSummary(err))
		return nil, false
	}

	tenantDir := filepath.Join(baseOutput, tenantDomain)
	meta, err := docs.LoadExportMetadata(tenantDir)
	if err != nil {
		if errors.Is(err, docs.ErrNoMetadata) {
			log.Debug("No export metadata for tenant", "tenant", tenantDomain)
		} else {
			log.Warn("Could not read the export metadata; display names are not shown", "error", err)
		}
		return nil, false
	}

	names := make(map[string]string, len(meta.Resources))
	for _, rm := range meta.Resources {
		if rm.ResourceId == "" || rm.DisplayName == "" {
			continue
		}
		names[strings.ToLower(rm.ResourceId)] = rm.DisplayName
	}
	log.Info("Display names joined from the existing export", "export", tenantDir)
	return names, true
}
