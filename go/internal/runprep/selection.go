package runprep

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/viper"
)

// resourceGroupType is the type a --resource-group selection acts on.
const resourceGroupType = "Microsoft.Resources/resourceGroups"

// ErrUnknownExcludedType is returned when the tenant profile's exclude-type
// names a type this build does not register. A typo would otherwise exclude
// nothing while the operator believes the type is left out.
var ErrUnknownExcludedType = errors.New("exclude-type names an unknown resource type")

// ErrExcludedTypeSelected is returned when the run's selection names a type the
// tenant profile excludes. The profile is the tenant's record; a one-off flag
// or a general type: list does not override it.
var ErrExcludedTypeSelected = errors.New("the selection names a resource type the tenant profile excludes")

// ErrEverythingExcluded is returned when the exclusion leaves no registered
// type to list. An empty type list means "every type" to the listing, so it
// must never reach it.
var ErrEverythingExcluded = errors.New("exclude-type leaves no resource type to list")

// TypeSelection is the outcome of resolving a run's type selection against the
// tenant's exclusion.
type TypeSelection struct {
	// Excluded is the tenant's exclusion in the registered spelling, sorted
	// and free of duplicates. It is recorded in the export metadata on every
	// run, because it belongs to the profile, not to the selection.
	Excluded []string
	// Effective is the list of types to enumerate: the allow-list as asked,
	// else every registered type (sorted), minus the exclusion.
	Effective []string
}

// SelectTypes resolves which resource types a run lists. registered are the
// type names the registry knows; allowList is --type, else the configured
// type: list; excluded is the tenant profile's exclude-type; resourceIDs and
// resourceGroup are the ARM selectors; profile names the profile file for
// error messages.
//
// Each excluded name is matched case-insensitively against the registered
// types and normalised to the registered spelling; an unknown name is an
// error. A selection that names an excluded type — an allow-list entry, the
// type parsed from a --resource-id, or the resource-group type of a
// --resource-group run — refuses rather than silently dropping it. IDs that do
// not parse to a type (bare Microsoft Graph GUIDs) carry no type and pass.
//
// It does no I/O, so every command that lists can call it before signing in.
func SelectTypes(registered, allowList, excluded, resourceIDs []string, resourceGroup, profile string) (TypeSelection, error) {
	var sel TypeSelection

	byLower := make(map[string]string, len(registered))
	for _, t := range registered {
		byLower[strings.ToLower(t)] = t
	}

	excludedSet := map[string]bool{}
	for _, name := range excluded {
		name = strings.TrimSpace(name)
		canonical, ok := byLower[strings.ToLower(name)]
		if !ok {
			return sel, fmt.Errorf("%w: %q in %s (run 'azure-rd resource types' for the registered names)",
				ErrUnknownExcludedType, name, profile)
		}
		if excludedSet[strings.ToLower(canonical)] {
			continue
		}
		excludedSet[strings.ToLower(canonical)] = true
		sel.Excluded = append(sel.Excluded, canonical)
	}
	sort.Strings(sel.Excluded)

	if err := refuseExcludedSelection(excludedSet, allowList, resourceIDs, resourceGroup, profile); err != nil {
		return sel, err
	}

	candidates := allowList
	if len(candidates) == 0 {
		candidates = append([]string(nil), registered...)
		sort.Strings(candidates)
	}
	for _, t := range candidates {
		if !excludedSet[strings.ToLower(t)] {
			sel.Effective = append(sel.Effective, t)
		}
	}
	if len(sel.Effective) == 0 && len(candidates) > 0 {
		return sel, fmt.Errorf("%w (%s)", ErrEverythingExcluded, profile)
	}
	return sel, nil
}

// refuseExcludedSelection returns ErrExcludedTypeSelected for the first
// selected type the exclusion names: an allow-list entry, and the types derived
// from the ARM selectors the way SelectedTypeNames derives them.
func refuseExcludedSelection(excludedSet map[string]bool, allowList, resourceIDs []string, resourceGroup, profile string) error {
	if len(excludedSet) == 0 {
		return nil
	}
	refuse := func(t, how string) error {
		return fmt.Errorf("%w: %s names %q, which exclude-type in %s excludes; remove it from one of them",
			ErrExcludedTypeSelected, how, t, profile)
	}
	for _, t := range allowList {
		if excludedSet[strings.ToLower(t)] {
			return refuse(t, "--type (or the configured type: list)")
		}
	}
	switch {
	case len(resourceIDs) > 0:
		if id, fullType, ok := excludedResourceID(excludedSet, resourceIDs); ok {
			return refuse(fullType, fmt.Sprintf("--resource-id %q", id))
		}
	case resourceGroup != "":
		if excludedSet[strings.ToLower(resourceGroupType)] {
			return refuse(resourceGroupType, "--resource-group")
		}
	}
	return nil
}

// excludedResourceID returns the first resource id whose parsed type the
// exclusion names; ids that do not parse are ignored here.
func excludedResourceID(excludedSet map[string]bool, resourceIDs []string) (id, fullType string, found bool) {
	for _, id := range resourceIDs {
		info, err := azure.ParseResourceID(id)
		if err != nil || info.FullType == "" {
			continue
		}
		if excludedSet[strings.ToLower(info.FullType)] {
			return id, info.FullType, true
		}
	}
	return "", "", false
}

// SelectTypesFromConfig resolves the selection against the registered types
// and the tenant profile's exclude-type, and logs a non-empty exclusion once.
// It is the one entry point every command that lists goes through (download
// and drift via Prepare, list and types directly), so they cannot disagree
// about which types a tenant leaves out.
func SelectTypesFromConfig(registered, allowList, resourceIDs []string, resourceGroup string) (TypeSelection, error) {
	excluded, profile := ExcludedTypesFromConfig()
	sel, err := SelectTypes(registered, allowList, excluded, resourceIDs, resourceGroup, profile)
	if err != nil {
		return sel, err
	}
	if len(sel.Excluded) > 0 {
		logger.Default.Info("Resource types excluded by the tenant profile", "types", sel.Excluded, "profile", profile)
	}
	return sel, nil
}

// ExcludedTypesFromConfig returns the tenant profile's exclude-type list and a
// label naming the profile it came from, for error messages. The key is
// tenant-scoped, so a non-empty value can only have come from the profile,
// which config.Load merges last — the file viper last read.
func ExcludedTypesFromConfig() (excluded []string, profile string) {
	profile = viper.ConfigFileUsed()
	if profile == "" {
		profile = "the tenant profile"
	}
	return viper.GetStringSlice(config.ExcludeTypeKey), profile
}
