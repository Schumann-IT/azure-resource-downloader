package resource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/runprep"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewTypesCommand builds the `resource types` command: the map of what this
// binary can handle. The map itself is a property of the build — answerable
// offline — and is enriched with live tenant counts only when a usable session
// happens to be available, decided without ever prompting.
func NewTypesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "Show the resource types this build supports",
		Long: `Show every Azure resource type this build can handle: the handler that
implements it and the API it speaks, grouped by API surface. This is a property
of the binary — it needs no subscription, no sign-in and no network — and is the
reference for --type values.

When a usable session is available the map is enriched with how many resources
of each type the tenant holds, counted by the same listing the other resource
commands use. The session is probed without any interaction: a sign-in that
would require a prompt (device-code, browser) counts as "not available" and the
command prints the offline map with a note instead. A type whose listing was
refused is reported as unknown — never as 0, which would mean "listed and found
nothing".

Selection flags narrow the map offline and the counting online. This command
writes nothing, so --dry-run changes nothing.

Examples:
  # The full supported-type map (works offline)
  azure-rd resource types

  # Only one type, with its tenant count when signed in
  azure-rd resource types --type "Microsoft.Graph/groups"`,
		RunE: runTypes,
	}
}

func runTypes(cmd *cobra.Command, args []string) error {
	// Bind the flags that apply to this command (inherited from the resource
	// group and root) to viper before reading any values so the
	// flag > env > config > default precedence holds without a sibling command
	// stealing the binding.
	cmdutil.BindFlags(cmd)

	ctx := cmd.Context()
	log := logger.Default

	sub := viper.GetString("subscription")
	selectedTypes := viper.GetStringSlice("type")
	resourceGroup := viper.GetString("resource-group")
	resourceIDs := viper.GetStringSlice("resource-id")
	workersFlag := viper.GetInt("workers")
	workersExplicit := cmd.Flags().Changed("workers")

	// This command has no output artifact, so there is nothing for --dry-run to
	// withhold; say so instead of silently ignoring the flag.
	if viper.GetBool("dry-run") {
		log.Info("--dry-run has no effect here: this command writes nothing either way")
	}

	// The supported types are baked into the binary, so the map needs neither a
	// subscription nor a signed-in session. Build a lazy credential (no network,
	// no token fetch until first use) purely so the registry's handler
	// constructors succeed — the same offline path download uses for its probe
	// registry. This keeps `azure-rd resource types` usable as documentation,
	// even offline or without 'az login'.
	lazyCred, err := azure.NewCredential("", "")
	if err != nil {
		return fmt.Errorf("failed to prepare Azure credentials: %w", err)
	}
	registry := handlers.NewRegistry(lazyCred, sub, false)

	// Resolve the selection to type names through the same precedence the
	// download's listing uses, then keep only registered types: an unregistered
	// --type is a reporting matter here, not a hard error as it would be for a
	// download.
	var types []string
	for _, t := range runprep.SelectedTypeNames(registry, selectedTypes, resourceGroup, resourceIDs) {
		if !registry.HasHandler(t) {
			log.Warn("Unsupported resource type, excluded from the map", "type", t)
			continue
		}
		types = append(types, t)
	}
	sort.Strings(types)

	// Enrich with tenant counts when — and only when — a session is available
	// without interaction. Failing to obtain one is not an error: the offline
	// map is complete and correct on its own terms.
	workerConfig := runprep.BuildWorkerConfig(workersExplicit)
	counts, unknown, omitReason := tenantCounts(ctx, sub, viper.GetString("client-id"), viper.GetString("tenant-id"),
		types, runprep.ListingConcurrency(workerConfig, workersFlag, workersExplicit))

	// State which of the two outputs this is: the same invocation prints
	// different things on different machines, and an operator must never infer
	// an empty tenant from an absent column.
	log.Info("Supported Azure resource types", "count", len(types))
	if counts == nil {
		log.Info("Tenant counts omitted (no usable session without prompting); showing the offline type map only",
			"reason", omitReason)
	} else {
		log.Info("Tenant counts included from a live listing", "types_unknown", len(unknown))
	}

	// Group the map by API surface so the ARM and Microsoft Graph halves are
	// distinguishable at a glance. The columns are identical in both modes; the
	// count is an added column, never a different layout.
	byAPI := map[models.APIType][]string{}
	for _, t := range types {
		api := models.DetectAPIType(t)
		byAPI[api] = append(byAPI[api], t)
	}
	for _, api := range []models.APIType{models.APIAzureResourceManager, models.APIMicrosoftGraph} {
		grouped := byAPI[api]
		if len(grouped) == 0 {
			continue
		}
		log.Info(fmt.Sprintf("%s (%d types)", api, len(grouped)))
		for _, t := range grouped {
			handler, err := registry.Get(t)
			if err != nil {
				// Cannot happen: types was filtered to registered handlers above.
				return fmt.Errorf("no handler registered for resource type %s: %w", t, err)
			}
			kv := []interface{}{"handler", handlerIdentity(handler)}
			if counts != nil {
				if reason, isUnknown := unknown[t]; isUnknown {
					// An unlistable type has no count, and zero is not it.
					kv = append(kv, "count", "unknown", "reason", reason)
				} else {
					kv = append(kv, "count", counts[t])
				}
			}
			log.Info(t, kv...)
		}
	}

	// Unknown counts and omitted counts are notes, never failures: a missing
	// session or privilege warns, it does not fail the run.
	return nil
}

// tenantCounts rolls the tenant's resources up per type — the same listing
// BuildFetchRequests performs for a download, rendered as a per-type view — for
// exactly the given registered types. It never prompts and never fails: when no
// token is obtainable without interaction (or anything else goes wrong), it
// returns nil counts and the reason, and the caller degrades to the offline
// map. Types whose listing was refused are returned in unknown with the reason
// and have no count; a type that listed to zero resources counts as 0.
func tenantCounts(ctx context.Context, sub, clientID, tenantID string, types []string, listConcurrency int) (map[string]int, map[string]string, string) {
	log := logger.Default

	if len(types) == 0 {
		return nil, nil, "no types selected"
	}

	// The non-interactive variant of the credential the other resource commands
	// use: the Azure CLI credential fails fast without a session, and the
	// device-code credential is constructed so a token request that would start
	// the sign-in flow returns an authentication-required error instead.
	cred, err := azure.NewNonInteractiveCredential(clientID, tenantID)
	if err != nil {
		return nil, nil, azure.ErrorSummary(err)
	}
	ok, reason := azure.ProbeSession(ctx, cred)
	if !ok {
		return nil, nil, reason
	}

	// A session exists: resolve the subscription (its absence only skips ARM
	// types, exactly as it would for a download) and list through the shared
	// path, so these counts can never disagree with what a download would fetch.
	client, err := azure.NewClientWithCredential(ctx, cred, sub, tenantID)
	if err != nil {
		log.Debug("Azure client construction failed", "error", err)
		return nil, nil, azure.ErrorSummary(err)
	}
	registry := handlers.NewRegistry(client.GetCredential(), client.GetSubscriptionID(), false)
	requests, skippedTypes, _, err := registry.BuildFetchRequests(ctx, nil, "", types, client.GetSubscriptionID(), listConcurrency)
	if err != nil {
		log.Debug("Listing for tenant counts failed", "error", err)
		return nil, nil, azure.ErrorSummary(err)
	}

	// Every selected type starts at 0 (a successful listing with no resources
	// is a real zero), then unlistable types are moved to unknown so they can
	// never render as 0.
	counts := make(map[string]int, len(types))
	for _, t := range types {
		counts[t] = 0
	}
	unknown := make(map[string]string, len(skippedTypes))
	for _, st := range skippedTypes {
		unknown[st.ResourceType] = st.Reason
		delete(counts, st.ResourceType)
	}
	for _, r := range requests {
		counts[r.ResourceType]++
	}
	return counts, unknown, ""
}

// handlerIdentity derives a printable handler name from the registered
// handler's Go type (e.g. "graph.GraphCollectionHandler"), so the map needs no
// new ResourceHandler method: the registry already knows the implementation.
func handlerIdentity(h models.ResourceHandler) string {
	return strings.TrimPrefix(fmt.Sprintf("%T", h), "*")
}
