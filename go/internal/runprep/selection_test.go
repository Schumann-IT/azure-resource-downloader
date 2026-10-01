package runprep

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"

	"github.com/spf13/viper"
)

// registeredForTest is a small registry snapshot, deliberately unsorted like
// Registry.GetAllTypes.
var registeredForTest = []string{
	"Microsoft.Graph/groups",
	"Microsoft.Compute/virtualMachines",
	"Microsoft.Resources/resourceGroups",
	"Microsoft.Storage/storageAccounts",
	"Microsoft.Graph/namedLocations",
}

const testProfile = "/cfg/contoso.example.com.yaml"

func TestSelectTypes(t *testing.T) {
	armTypes := []string{"Microsoft.Compute/virtualMachines", "Microsoft.Storage/storageAccounts", "Microsoft.Resources/resourceGroups"}
	vmID := "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm1"
	storageID := "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/acct"

	tests := []struct {
		name          string
		allow         []string
		excluded      []string
		ids           []string
		group         string
		wantExcluded  []string
		wantEffective []string
		wantErr       error
		wantInErr     []string
	}{
		{
			name: "no exclusion lists every registered type, sorted",
			wantEffective: []string{
				"Microsoft.Compute/virtualMachines", "Microsoft.Graph/groups", "Microsoft.Graph/namedLocations",
				"Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts",
			},
		},
		{
			name:          "all registered minus the excluded",
			excluded:      armTypes,
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines", "Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts"},
			wantEffective: []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"},
		},
		{
			name:          "the allow-list minus the excluded keeps its order",
			allow:         []string{"Microsoft.Graph/namedLocations", "Microsoft.Graph/groups"},
			excluded:      []string{"Microsoft.Compute/virtualMachines"},
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines"},
			wantEffective: []string{"Microsoft.Graph/namedLocations", "Microsoft.Graph/groups"},
		},
		{
			name:          "excluded names are normalised case-insensitively and duplicates collapse",
			excluded:      []string{"microsoft.compute/VIRTUALMACHINES", "Microsoft.Compute/virtualMachines", " microsoft.storage/storageaccounts "},
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines", "Microsoft.Storage/storageAccounts"},
			wantEffective: []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations", "Microsoft.Resources/resourceGroups"},
		},
		{
			name:      "an unknown excluded name errors naming the type and the profile",
			excluded:  []string{"Microsoft.Compute/virtualMachine"},
			wantErr:   ErrUnknownExcludedType,
			wantInErr: []string{"Microsoft.Compute/virtualMachine", testProfile},
		},
		{
			name:      "an allow-list entry that is excluded refuses",
			allow:     []string{"Microsoft.Graph/groups", "microsoft.compute/virtualmachines"},
			excluded:  []string{"Microsoft.Compute/virtualMachines"},
			wantErr:   ErrExcludedTypeSelected,
			wantInErr: []string{"microsoft.compute/virtualmachines", testProfile},
		},
		{
			name:      "a --resource-id of an excluded type refuses",
			excluded:  armTypes,
			ids:       []string{"00000000-0000-0000-0000-000000000001", vmID},
			wantErr:   ErrExcludedTypeSelected,
			wantInErr: []string{"Microsoft.Compute/virtualMachines", testProfile, vmID},
		},
		{
			name:      "--resource-group while resourceGroups is excluded refuses",
			excluded:  armTypes,
			group:     "rg",
			wantErr:   ErrExcludedTypeSelected,
			wantInErr: []string{"Microsoft.Resources/resourceGroups", "--resource-group", testProfile},
		},
		{
			name:          "an unparseable id carries no type and passes",
			excluded:      armTypes,
			ids:           []string{"00000000-0000-0000-0000-000000000001"},
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines", "Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts"},
			wantEffective: []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"},
		},
		{
			name:          "an unrelated id passes",
			excluded:      []string{"Microsoft.Compute/virtualMachines"},
			ids:           []string{storageID},
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines"},
			wantEffective: []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations", "Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts"},
		},
		{
			name:          "an unrelated group passes",
			excluded:      []string{"Microsoft.Compute/virtualMachines"},
			group:         "rg",
			wantExcluded:  []string{"Microsoft.Compute/virtualMachines"},
			wantEffective: []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations", "Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts"},
		},
		{
			name:     "excluding everything refuses instead of listing everything",
			excluded: registeredForTest,
			wantErr:  ErrEverythingExcluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectTypes(registeredForTest, tt.allow, tt.excluded, tt.ids, tt.group, testProfile)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SelectTypes() error = %v, want %v", err, tt.wantErr)
				}
				for _, want := range tt.wantInErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q should name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectTypes() error = %v", err)
			}
			if !reflect.DeepEqual(got.Excluded, tt.wantExcluded) {
				t.Errorf("Excluded = %v, want %v", got.Excluded, tt.wantExcluded)
			}
			if !reflect.DeepEqual(got.Effective, tt.wantEffective) {
				t.Errorf("Effective = %v, want %v", got.Effective, tt.wantEffective)
			}
		})
	}
}

// TestSelectedTypeNamesFedTheEffectiveListHasNoExcludedType: the dedicated-app
// probe is fed the effective list, so the scope prompt never asks for an
// excluded type's permissions.
func TestSelectedTypeNamesFedTheEffectiveListHasNoExcludedType(t *testing.T) {
	excluded := []string{"Microsoft.Compute/virtualMachines", "Microsoft.Graph/namedLocations"}
	sel, err := SelectTypes(registeredForTest, nil, excluded, nil, "", testProfile)
	if err != nil {
		t.Fatalf("SelectTypes() error = %v", err)
	}
	for _, typ := range SelectedTypeNames(nil, sel.Effective, "", nil) {
		for _, ex := range excluded {
			if strings.EqualFold(typ, ex) {
				t.Errorf("SelectedTypeNames() contains excluded type %q", typ)
			}
		}
	}
}

// listingHandler is a handler whose listing returns one id and records that it
// was asked; an excluded type must never be asked.
type listingHandler struct {
	typ    string
	listed *sync.Map
}

func (h listingHandler) GetType() string                { return h.typ }
func (h listingHandler) GetDocumentationPrompt() string { return "" }
func (h listingHandler) List(context.Context) ([]string, error) {
	h.listed.Store(h.typ, true)
	return []string{"/" + h.typ + "/one"}, nil
}
func (h listingHandler) Fetch(context.Context, string) (interface{}, error) { return nil, nil }
func (h listingHandler) Transform(interface{}) (*models.TransformedResource, error) {
	return &models.TransformedResource{}, nil
}

// TestPreparedResolveSelectionAndBuildRequests: the profile's exclusion is
// read from the configuration, normalised, and the listing never asks an
// excluded type — it is neither covered nor skipped.
func TestPreparedResolveSelectionAndBuildRequests(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set(config.ExcludeTypeKey, []string{"microsoft.compute/virtualmachines", "Microsoft.Storage/storageAccounts"})

	listed := &sync.Map{}
	registry := handlers.NewEmptyRegistry()
	for _, typ := range registeredForTest {
		registry.Register(typ, listingHandler{typ: typ, listed: listed})
	}

	p := &Prepared{Registry: registry, Subscription: "sub", WorkerConfig: models.DefaultWorkerConfig()}
	if err := p.resolveSelection(registry.GetAllTypes()); err != nil {
		t.Fatalf("resolveSelection() error = %v", err)
	}
	wantExcluded := []string{"Microsoft.Compute/virtualMachines", "Microsoft.Storage/storageAccounts"}
	if !reflect.DeepEqual(p.ExcludedTypes, wantExcluded) {
		t.Errorf("ExcludedTypes = %v, want %v", p.ExcludedTypes, wantExcluded)
	}
	if len(p.SelectedTypes) != 0 {
		t.Errorf("SelectedTypes = %v, want the allow-list as asked (empty)", p.SelectedTypes)
	}

	requests, skipped, _, err := p.BuildRequests(context.Background())
	if err != nil {
		t.Fatalf("BuildRequests() error = %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none (an excluded type is never requested, so never skipped)", skipped)
	}
	for _, ex := range wantExcluded {
		if _, ok := listed.Load(ex); ok {
			t.Errorf("excluded type %q was listed", ex)
		}
	}
	if got, want := len(requests), len(registeredForTest)-len(wantExcluded); got != want {
		t.Errorf("requests = %d, want %d (one per effective type)", got, want)
	}

	viper.Set("type", []string{"Microsoft.Compute/virtualMachines"})
	clash := &Prepared{SelectedTypes: viper.GetStringSlice("type")}
	if err := clash.resolveSelection(registry.GetAllTypes()); !errors.Is(err, ErrExcludedTypeSelected) {
		t.Errorf("resolveSelection() with an excluded --type error = %v, want ErrExcludedTypeSelected", err)
	}
}
