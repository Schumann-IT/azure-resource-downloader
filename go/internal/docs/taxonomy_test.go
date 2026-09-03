package docs

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
)

// validTaxonomy is a small, valid taxonomy exercising every rule key and the
// OR-of-rules / AND-within-a-rule semantics, plus a programme that matches
// nothing in the scenarios below (its count must still be emitted as zero).
func validTaxonomy() TaxonomyConfig {
	return TaxonomyConfig{
		Version: 1,
		Programmes: []TaxonomyProgramme{
			{
				ID:    "cis-l1",
				Label: "CIS L1 hardening",
				Match: []TaxonomyRule{
					{Name: `_cis_.*_l1`},
					{Type: "Microsoft.Graph/deviceShellScripts", Name: "cis"},
				},
			},
			{
				ID:    "windows-updates",
				Label: "Windows Update rings",
				Match: []TaxonomyRule{
					{Platforms: "windows", Scope: "device"},
				},
			},
			{
				ID:    "vpn",
				Label: "VPN",
				Match: []TaxonomyRule{
					{ODataType: `vpnConfiguration`},
				},
			},
		},
	}
}

func TestCompileTaxonomyValid(t *testing.T) {
	tax, err := compileTaxonomy(validTaxonomy())
	if err != nil {
		t.Fatalf("compileTaxonomy: %v", err)
	}
	got := tax.registry()
	want := []string{"cis-l1", "windows-updates", "vpn"}
	if len(got) != len(want) {
		t.Fatalf("registry length = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].id != w {
			t.Errorf("registry[%d].id = %q, want %q (order must be preserved)", i, got[i].id, w)
		}
	}
}

func TestCompileTaxonomyErrors(t *testing.T) {
	tests := []struct {
		name string
		tf   TaxonomyConfig
	}{
		{
			name: "version below one",
			tf:   TaxonomyConfig{Version: 0, Programmes: []TaxonomyProgramme{{ID: "a", Label: "A", Match: []TaxonomyRule{{Name: "x"}}}}},
		},
		{
			name: "invalid id",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "Not Safe", Label: "A", Match: []TaxonomyRule{{Name: "x"}}}}},
		},
		{
			name: "duplicate id",
			tf: TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{
				{ID: "a", Label: "A", Match: []TaxonomyRule{{Name: "x"}}},
				{ID: "a", Label: "B", Match: []TaxonomyRule{{Name: "y"}}},
			}},
		},
		{
			name: "missing label",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "a", Match: []TaxonomyRule{{Name: "x"}}}}},
		},
		{
			name: "no rules",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "a", Label: "A"}}},
		},
		{
			name: "empty rule",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "a", Label: "A", Match: []TaxonomyRule{{}}}}},
		},
		{
			name: "bad name regex",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "a", Label: "A", Match: []TaxonomyRule{{Name: "("}}}}},
		},
		{
			name: "bad scope",
			tf:   TaxonomyConfig{Version: 1, Programmes: []TaxonomyProgramme{{ID: "a", Label: "A", Match: []TaxonomyRule{{Scope: "group"}}}}},
		},
		{
			name: "programmes and programme axis collision",
			tf: TaxonomyConfig{Version: 1,
				Programmes: []TaxonomyProgramme{{ID: "a", Label: "A", Match: []TaxonomyRule{{Name: "x"}}}},
				Axes:       []TaxonomyAxis{{ID: "programme", Label: "P", Values: []TaxonomyValue{{ID: "b", Label: "B", Match: []TaxonomyRule{{Name: "y"}}}}}},
			},
		},
		{
			name: "invalid axis id",
			tf:   TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "Not Safe", Label: "P", Values: []TaxonomyValue{{ID: "b", Label: "B", Match: []TaxonomyRule{{Name: "y"}}}}}}},
		},
		{
			name: "duplicate axis id",
			tf: TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{
				{ID: "p", Label: "P", Values: []TaxonomyValue{{ID: "b", Label: "B", Match: []TaxonomyRule{{Name: "y"}}}}},
				{ID: "p", Label: "Q", Values: []TaxonomyValue{{ID: "c", Label: "C", Match: []TaxonomyRule{{Name: "z"}}}}},
			}},
		},
		{
			name: "axis without label",
			tf:   TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "p", Values: []TaxonomyValue{{ID: "b", Label: "B", Match: []TaxonomyRule{{Name: "y"}}}}}}},
		},
		{
			name: "axis without values",
			tf:   TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "p", Label: "P"}}},
		},
		{
			name: "invalid value id",
			tf:   TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "p", Label: "P", Values: []TaxonomyValue{{ID: "Bad ID", Label: "B", Match: []TaxonomyRule{{Name: "y"}}}}}}},
		},
		{
			name: "duplicate value id within axis",
			tf: TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "p", Label: "P", Values: []TaxonomyValue{
				{ID: "b", Label: "B", Match: []TaxonomyRule{{Name: "y"}}},
				{ID: "b", Label: "C", Match: []TaxonomyRule{{Name: "z"}}},
			}}}},
		},
		{
			name: "value without rules",
			tf:   TaxonomyConfig{Version: 1, Axes: []TaxonomyAxis{{ID: "p", Label: "P", Values: []TaxonomyValue{{ID: "b", Label: "B"}}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compileTaxonomy(tt.tf); err == nil {
				t.Errorf("expected an error for %s, got nil", tt.name)
			}
		})
	}
}

func TestTaxonomyClassify(t *testing.T) {
	tax, err := compileTaxonomy(validTaxonomy())
	if err != nil {
		t.Fatalf("compileTaxonomy: %v", err)
	}

	tests := []struct {
		name  string
		facts taxonomyFacts
		want  []string
	}{
		{
			name:  "name regex, case-insensitive",
			facts: taxonomyFacts{name: "GBL_AF_PRD_D_WIN_CIS_OS_L1", platforms: "windows", scope: "device"},
			// matches cis-l1 (name) and windows-updates (platform+scope), in
			// registry order.
			want: []string{"cis-l1", "windows-updates"},
		},
		{
			name:  "AND within a rule: type plus name",
			facts: taxonomyFacts{name: "Mac CIS baseline", rtype: "Microsoft.Graph/deviceShellScripts"},
			want:  []string{"cis-l1"},
		},
		{
			name:  "AND fails when one field mismatches",
			facts: taxonomyFacts{name: "Mac CIS baseline", rtype: "Microsoft.Graph/deviceManagementScripts"},
			want:  nil,
		},
		{
			name:  "scope must match exactly",
			facts: taxonomyFacts{platforms: "windows", scope: "user"},
			want:  nil,
		},
		{
			name:  "odataType regex",
			facts: taxonomyFacts{odataType: "#microsoft.graph.windows81VpnConfiguration"},
			want:  []string{"vpn"},
		},
		{
			name:  "no match is uncategorised",
			facts: taxonomyFacts{name: "Some Android app", platforms: "android"},
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tax.classify(tt.facts)
			if len(got) != len(tt.want) {
				t.Fatalf("classify = %v, want %v", ids(got), tt.want)
			}
			for i, w := range tt.want {
				if got[i].id != w {
					t.Errorf("classify[%d] = %q, want %q", i, got[i].id, w)
				}
			}
		})
	}
}

// TestTaxonomyViperRoundTrip guards the viper gotcha: viper lowercases config
// keys, so the `odataType` rule field arrives as `odatatype`. Reading with
// viper.UnmarshalKey (mapstructure, case-insensitive) must still populate it, or
// odataType rules would silently never match.
func TestTaxonomyViperRoundTrip(t *testing.T) {
	const cfg = `
taxonomy:
  version: 1
  programmes:
    - id: vpn
      label: VPN
      match:
        - odataType: vpnConfiguration
    - id: apps
      label: Apps
      match:
        - type: Microsoft.Graph/mobileApps
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewReader([]byte(cfg))); err != nil {
		t.Fatalf("read config: %v", err)
	}

	var tc TaxonomyConfig
	if err := v.UnmarshalKey("taxonomy", &tc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tax, err := compileTaxonomy(tc)
	if err != nil {
		t.Fatalf("compileTaxonomy: %v", err)
	}

	// The odataType rule survived viper's lowercasing.
	got := tax.classify(taxonomyFacts{odataType: "#microsoft.graph.windows81VpnConfiguration"})
	if len(got) != 1 || got[0].id != "vpn" {
		t.Errorf("odataType classify = %v, want [vpn]", ids(got))
	}
	// The type rule also round-tripped (type values are not lowercased).
	got = tax.classify(taxonomyFacts{rtype: "Microsoft.Graph/mobileApps"})
	if len(got) != 1 || got[0].id != "apps" {
		t.Errorf("type classify = %v, want [apps]", ids(got))
	}
}

// TestClassifyAxesTwoAxes exercises the generalised, multi-axis classification:
// the programmes sugar is prepended as the "programme" axis, explicit axes
// follow in config order, and every axis is represented in the result (an empty
// value slice means uncategorised on that axis).
func TestClassifyAxesTwoAxes(t *testing.T) {
	cfg := TaxonomyConfig{
		Version: 1,
		Programmes: []TaxonomyProgramme{
			{ID: "vpn", Label: "VPN", Match: []TaxonomyRule{{ODataType: "vpn"}}},
		},
		Axes: []TaxonomyAxis{
			{ID: "platform", Label: "Platform", Values: []TaxonomyValue{
				{ID: "windows", Label: "Windows", Match: []TaxonomyRule{{Platforms: "windows"}}},
				{ID: "macos", Label: "macOS", Match: []TaxonomyRule{{Platforms: "macos"}}},
			}},
		},
	}
	tax, err := compileTaxonomy(cfg)
	if err != nil {
		t.Fatalf("compileTaxonomy: %v", err)
	}

	got := tax.classifyAxes(taxonomyFacts{odataType: "#microsoft.graph.windows81VpnConfiguration", platforms: "windows"})
	if len(got) != 2 || got[0].axisID != "programme" || got[1].axisID != "platform" {
		t.Fatalf("axes = %+v, want [programme platform]", got)
	}
	if len(got[0].values) != 1 || got[0].values[0].id != "vpn" {
		t.Errorf("programme values = %v, want [vpn]", ids(got[0].values))
	}
	if len(got[1].values) != 1 || got[1].values[0].id != "windows" {
		t.Errorf("platform values = %v, want [windows]", ids(got[1].values))
	}

	// A resource matching neither axis is represented by empty value slices, so
	// the caller can count it uncategorised on each axis.
	none := tax.classifyAxes(taxonomyFacts{name: "nothing", platforms: "android"})
	if len(none) != 2 || len(none[0].values) != 0 || len(none[1].values) != 0 {
		t.Errorf("no-match classification = %+v, want two empty axes", none)
	}
}

// ids extracts the ids from a slice of programmeRef for readable assertions.
func ids(refs []programmeRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.id)
	}
	return out
}
