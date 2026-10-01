package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// The golden test proves that a dependency update is byte-neutral for the
// exported YAML. Each case parses a synthetic Microsoft Graph response with the
// SDK's own JSON parse-node factory (the one the production request adapter
// uses), runs it through the production transform stage and compares the bytes
// MarshalResourceYAML produces — the bytes the writer puts on disk and hashes
// into sourceSha256 — with a checked-in golden file. A Graph SDK, Kiota or YAML
// bump that changes any of them fails here, offline, before it could move the
// hash of resources that did not change in the tenant.
//
// Regenerating the golden files is a deliberate step: set UPDATE_GOLDEN=1
// (`make golden-update`) and review the diff. A missing golden file fails the
// test; it is never created implicitly.

// goldenDir holds the fixtures: <case>.json (a Graph response body) and
// <case>.golden.yaml (the expected exported YAML).
const goldenDir = "testdata/golden"

// updateGoldenEnv names the environment switch that rewrites the golden files
// instead of comparing against them.
const updateGoldenEnv = "UPDATE_GOLDEN"

// errOfflineCredential is what the golden test's credential returns: the test
// never sends a request, so a token request is a bug.
var errOfflineCredential = errors.New("golden test credential: no token in an offline test")

// offlineCredential is a test-local azcore.TokenCredential. It lets the
// production registry build its Graph clients — which registers the SDK's JSON
// parse-node factory with the backing store enabled — without any network.
type offlineCredential struct{}

func (offlineCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, errOfflineCredential
}

// goldenCase maps one resource type to its fixture and to the SDK model factory
// the production request builder passes for that endpoint.
type goldenCase struct {
	name         string
	resourceType string
	factory      serialization.ParsableFactory
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{
			name:         "conditional-access-policy",
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			factory:      msgraphmodels.CreateConditionalAccessPolicyFromDiscriminatorValue,
		},
		{
			name:         "group",
			resourceType: "Microsoft.Graph/groups",
			factory:      msgraphmodels.CreateGroupFromDiscriminatorValue,
		},
		{
			name:         "settings-catalog-policy",
			resourceType: "Microsoft.Graph/deviceManagementConfigurationPolicies",
			factory:      betamodels.CreateDeviceManagementConfigurationPolicyFromDiscriminatorValue,
		},
		{
			name:         "device-configuration-ios-custom",
			resourceType: "Microsoft.Graph/deviceConfigurations",
			factory:      betamodels.CreateDeviceConfigurationFromDiscriminatorValue,
		},
		{
			name:         "mobile-app-win32",
			resourceType: "Microsoft.Graph/mobileApps",
			factory:      betamodels.CreateMobileAppFromDiscriminatorValue,
		},
	}
}

func TestGoldenExportYAML(t *testing.T) {
	registry := handlers.NewRegistry(offlineCredential{}, "00000000-0000-0000-0000-000000000000", false)
	transformer := NewTransformer(registry, 1, models.DefaultTransformerConfigs(), nil)
	update := os.Getenv(updateGoldenEnv) != ""

	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := renderGoldenCase(t, transformer, tc)
			goldenPath := filepath.Join(goldenDir, tc.name+".golden.yaml")

			if update {
				if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
					t.Fatalf("case %s: write golden file: %v", tc.name, err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("case %s: read golden file (create it deliberately with %s=1 / make golden-update): %v", tc.name, updateGoldenEnv, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("case %s: exported YAML differs from %s\n%s", tc.name, goldenPath, firstDifference(want, got))
			}
		})
	}
}

// renderGoldenCase parses the case's fixture with the SDK's registered JSON
// parse-node factory, runs the production transform stage on the model and
// returns the bytes the writer would put on disk.
func renderGoldenCase(t *testing.T, transformer *Transformer, tc goldenCase) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(goldenDir, tc.name+".json"))
	if err != nil {
		t.Fatalf("case %s: read fixture: %v", tc.name, err)
	}

	root, err := serialization.DefaultParseNodeFactoryInstance.GetRootParseNode("application/json", body)
	if err != nil {
		t.Fatalf("case %s: root parse node: %v", tc.name, err)
	}
	parsed, err := root.GetObjectValue(tc.factory)
	if err != nil {
		t.Fatalf("case %s: parse fixture: %v", tc.name, err)
	}
	if parsed == nil {
		t.Fatalf("case %s: fixture parsed to nil", tc.name)
	}

	result := transformer.transformResource(&models.FetchResult{
		ResourceID:   "golden/" + tc.name,
		ResourceType: tc.resourceType,
		RawData:      parsed,
	})
	if result.Error != nil {
		t.Fatalf("case %s: transform: %v", tc.name, result.Error)
	}
	if result.Filtered || result.Skipped || result.Cancelled {
		t.Fatalf("case %s: transform did not produce a resource (filtered=%t skipped=%t cancelled=%t)",
			tc.name, result.Filtered, result.Skipped, result.Cancelled)
	}

	out, err := MarshalResourceYAML(result.CleanedData)
	if err != nil {
		t.Fatalf("case %s: marshal YAML: %v", tc.name, err)
	}
	return out
}

// firstDifference describes the first line at which got departs from want,
// with that line from both sides, so a failing bump names what moved.
func firstDifference(want, got []byte) string {
	// The final newline ends the last line rather than starting an empty one,
	// so a truncated file reports <end of file> instead of an empty line.
	wantLines := strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := lineAt(wantLines, i), lineAt(gotLines, i)
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n  want: %s\n  got:  %s", i+1, w, g)
		}
	}
	return "no line differs (byte-level difference only)"
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return fmt.Sprintf("%q", lines[i])
	}
	return "<end of file>"
}

func TestFirstDifference(t *testing.T) {
	tests := []struct {
		name string
		want string
		got  string
		has  string
	}{
		{name: "changed line", want: "a: 1\nb: 2\n", got: "a: 1\nb: 3\n", has: `line 2:`},
		{name: "got shorter", want: "a: 1\nb: 2\n", got: "a: 1\n", has: "<end of file>"},
		{name: "got longer", want: "a: 1\n", got: "a: 1\nb: 2\n", has: `"b: 2"`},
		{name: "identical lines", want: "a: 1\n", got: "a: 1\n", has: "byte-level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if d := firstDifference([]byte(tt.want), []byte(tt.got)); !strings.Contains(d, tt.has) {
				t.Errorf("firstDifference() = %q, want it to contain %q", d, tt.has)
			}
		})
	}
}
