package resource

import (
	"context"
	"reflect"
	"testing"

	"azure-resource-downloader/internal/models"
)

// fakeHandler is a minimal models.ResourceHandler used to pin down the derived
// handler identity without any Azure dependency.
type fakeHandler struct{}

func (fakeHandler) GetType() string                { return "Microsoft.Test/fakes" }
func (fakeHandler) GetDocumentationPrompt() string { return "" }
func (fakeHandler) List(context.Context) ([]string, error) {
	return nil, nil
}
func (fakeHandler) Fetch(context.Context, string) (interface{}, error) {
	return nil, nil
}
func (fakeHandler) Transform(interface{}) (*models.TransformedResource, error) {
	return nil, nil
}

// TestHandlerIdentity guards the decision to derive the handler column from the
// registered handler's Go type instead of widening the ResourceHandler
// interface: the derivation must be readable (package-qualified, no pointer
// noise) or that decision needs revisiting.
func TestHandlerIdentity(t *testing.T) {
	if got := handlerIdentity(fakeHandler{}); got != "resource.fakeHandler" {
		t.Errorf("handlerIdentity(value) = %q, want %q", got, "resource.fakeHandler")
	}
	if got := handlerIdentity(&fakeHandler{}); got != "resource.fakeHandler" {
		t.Errorf("handlerIdentity(pointer) = %q, want %q", got, "resource.fakeHandler")
	}
}

// TestTenantCountsNoTypes pins the earliest degradation of the counting path:
// with nothing selected there is nothing to count, and the caller must receive
// nil counts plus a reason for its omission note — never an error, because
// failing to count may never fail the command.
func TestTenantCountsNoTypes(t *testing.T) {
	counts, unknown, reason := tenantCounts(context.Background(), "", "", "", nil, 4)
	if counts != nil || unknown != nil {
		t.Errorf("tenantCounts() = %v, %v; want nil counts for an empty selection", counts, unknown)
	}
	if reason == "" {
		t.Error("tenantCounts() returned no omission reason for an empty selection")
	}
}

// TestTypesMarksAnExcludedTypeAndDoesNotCountIt: the map keeps an excluded
// type — it is the catalogue an operator picks names from — but marks it and
// never hands it to the live listing, so it can never report a refused 403.
func TestTypesMarksAnExcludedTypeAndDoesNotCountIt(t *testing.T) {
	const vm = "Microsoft.Compute/virtualMachines"
	types := []string{vm, "Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"}
	excluded := map[string]bool{vm: true}

	if got, want := countableTypes(types, excluded), []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"}; !reflect.DeepEqual(got, want) {
		t.Errorf("countableTypes() = %v, want %v", got, want)
	}

	// Everything excluded: nothing is countable and every row is marked.
	all := map[string]bool{vm: true, "Microsoft.Graph/groups": true, "Microsoft.Graph/namedLocations": true}
	if got := countableTypes(types, all); len(got) != 0 {
		t.Errorf("countableTypes(all excluded) = %v, want empty", got)
	}
	for _, typ := range types {
		want := []interface{}{"handler", "resource.fakeHandler", "excluded", "exclude-type"}
		if got := typeRow(typ, fakeHandler{}, all[typ], nil, nil); !reflect.DeepEqual(got, want) {
			t.Errorf("typeRow(%s, all excluded) = %v, want %v", typ, got, want)
		}
	}

	counts := map[string]int{"Microsoft.Graph/groups": 3}
	unknown := map[string]string{"Microsoft.Graph/namedLocations": "403"}
	tests := []struct {
		name     string
		typ      string
		excluded bool
		counts   map[string]int
		want     []interface{}
	}{
		{name: "excluded, with counts", typ: vm, excluded: true, counts: counts,
			want: []interface{}{"handler", "resource.fakeHandler", "excluded", "exclude-type"}},
		{name: "excluded, offline", typ: vm, excluded: true,
			want: []interface{}{"handler", "resource.fakeHandler", "excluded", "exclude-type"}},
		{name: "counted", typ: "Microsoft.Graph/groups", counts: counts,
			want: []interface{}{"handler", "resource.fakeHandler", "count", 3}},
		{name: "unknown", typ: "Microsoft.Graph/namedLocations", counts: counts,
			want: []interface{}{"handler", "resource.fakeHandler", "count", "unknown", "reason", "403"}},
		{name: "offline", typ: "Microsoft.Graph/groups",
			want: []interface{}{"handler", "resource.fakeHandler"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeRow(tt.typ, fakeHandler{}, tt.excluded, tt.counts, unknown); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("typeRow() = %v, want %v", got, tt.want)
			}
		})
	}
}
