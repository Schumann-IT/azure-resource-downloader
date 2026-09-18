package runprep

import (
	"reflect"
	"testing"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/models"
)

func TestSelectedTypeNames(t *testing.T) {
	// A resource-id selection derives the type from the ID.
	ids := []string{
		"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/acct",
		"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/acct2",
	}
	got := SelectedTypeNames(nil, nil, "", ids)
	if len(got) != 1 || got[0] != "Microsoft.Storage/storageAccounts" {
		t.Errorf("resource-id selection = %v, want [Microsoft.Storage/storageAccounts]", got)
	}

	// A resource-group selection maps to the ARM resource group type.
	got = SelectedTypeNames(nil, nil, "my-rg", nil)
	if len(got) != 1 || got[0] != "Microsoft.Resources/resourceGroups" {
		t.Errorf("resource-group selection = %v, want [Microsoft.Resources/resourceGroups]", got)
	}

	// An explicit --type selection is returned as-is.
	types := []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"}
	got = SelectedTypeNames(nil, types, "", nil)
	if !reflect.DeepEqual(got, types) {
		t.Errorf("--type selection = %v, want %v", got, types)
	}
}

func TestDetermineWorkerCount(t *testing.T) {
	wc := models.DefaultWorkerConfig()

	tests := []struct {
		name         string
		resourceType string
		workersFlag  int
		explicit     bool
		want         int
	}{
		{
			name:         "explicit flag wins even at the default value",
			resourceType: "Microsoft.Graph/groups",
			workersFlag:  cmdutil.DefaultWorkerCount,
			explicit:     true,
			want:         cmdutil.DefaultWorkerCount,
		},
		{
			name:         "explicit non-default flag wins",
			resourceType: "Microsoft.Storage/storageAccounts",
			workersFlag:  17,
			explicit:     true,
			want:         17,
		},
		{
			name:         "no flag uses API-specific default for a single type",
			resourceType: "Microsoft.Storage/storageAccounts",
			workersFlag:  cmdutil.DefaultWorkerCount,
			explicit:     false,
			want:         wc.GetWorkerCount("Microsoft.Storage/storageAccounts"),
		},
		{
			name:         "no flag and mixed types uses the safe default",
			resourceType: "",
			workersFlag:  cmdutil.DefaultWorkerCount,
			explicit:     false,
			want:         wc.Default,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineWorkerCount(wc, tt.resourceType, tt.workersFlag, tt.explicit)
			if got != tt.want {
				t.Errorf("DetermineWorkerCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestListingConcurrency guards the one shared derivation of listing
// concurrency: an explicit --workers must mean the same thing in download,
// drift, list and types, and without it the Microsoft Graph worker count (the
// stricter rate limit) bounds the per-type listing calls.
func TestListingConcurrency(t *testing.T) {
	wc := models.DefaultWorkerConfig()

	if got := ListingConcurrency(wc, 3, true); got != 3 {
		t.Errorf("explicit --workers 3 yields %d, want 3", got)
	}
	if got := ListingConcurrency(wc, cmdutil.DefaultWorkerCount, false); got != wc.MicrosoftGraph {
		t.Errorf("no explicit flag yields %d, want the Graph worker count %d", got, wc.MicrosoftGraph)
	}
	sparse := &models.WorkerConfig{Default: 7}
	if got := ListingConcurrency(sparse, 0, false); got != 7 {
		t.Errorf("no Graph count yields %d, want the general default 7", got)
	}
}

// TestPreparedEffectiveType guards the "single type or mixed" decision worker
// tuning rests on.
func TestPreparedEffectiveType(t *testing.T) {
	p := &Prepared{SelectedTypes: []string{"Microsoft.Graph/groups"}}
	if got := p.EffectiveType(); got != "Microsoft.Graph/groups" {
		t.Errorf("EffectiveType() = %q, want the single selected type", got)
	}
	p.SelectedTypes = []string{"a", "b"}
	if got := p.EffectiveType(); got != "" {
		t.Errorf("EffectiveType() = %q, want \"\" for a mixed selection", got)
	}
}
