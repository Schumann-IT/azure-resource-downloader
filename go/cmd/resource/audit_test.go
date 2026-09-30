package resource

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/audit"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	auditTenant    = "contoso.example.com"
	auditWorkspace = "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"
	auditBaseline  = "2026-09-28T00:00:00Z"
)

type stubCredential struct{}

func (stubCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "stub", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// emptyQuerier answers every retention probe with an old earliest row and
// every event query with no rows.
type emptyQuerier struct{}

func (emptyQuerier) Query(_ context.Context, _ string, kql string, _, _ *time.Time) ([]audit.Row, error) {
	if strings.Contains(kql, "summarize") {
		return []audit.Row{{"Earliest": "2020-01-01T00:00:00Z"}}, nil
	}
	return nil, nil
}

// seedBaseline writes <base>/<tenant>/resources/metadata.yaml.
func seedBaseline(t *testing.T, base, generatedAt string) string {
	t.Helper()
	tenantDir := filepath.Join(base, auditTenant)
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&docs.Metadata{Tenant: auditTenant, GeneratedAt: generatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourcesDir, docs.MetadataFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return tenantDir
}

// seedObservation writes an observation decided against baselineGeneratedAt,
// through the real writer, and returns the stamped copy.
func seedObservation(t *testing.T, tenantDir, baselineGeneratedAt string) drift.Observation {
	t.Helper()
	rep := &drift.Report{
		Observation: drift.Observation{
			Tenant:   auditTenant,
			Baseline: drift.BaselineRef{GeneratedAt: baselineGeneratedAt},
			Findings: map[string]drift.Finding{
				"Microsoft.Graph/groups/g.yaml": {Verdict: drift.VerdictChanged, ResourceID: "11111111-2222-3333-4444-555555555555"},
			},
		},
		PayloadData: map[string][]byte{},
	}
	obs, _, err := drift.WriteObservation(tenantDir, rep, time.Date(2026, 9, 30, 9, 30, 0, 0, time.UTC), "test", false)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

// auditCommand builds `resource audit` with an explicit --domain, so the run
// resolves its export offline and never signs in.
func auditCommand(t *testing.T, output, workspace string) *cobra.Command {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("output", output)
	if workspace != "" {
		viper.Set(config.AuditWorkspaceKey, workspace)
	}
	cmd := NewAuditCommand()
	cmd.Flags().String("domain", "", "")
	if err := cmd.Flags().Set("domain", auditTenant); err != nil {
		t.Fatal(err)
	}
	cmd.SetContext(context.Background())
	return cmd
}

// TestAuditCannotAnswer: every "nothing could be answered" case exits 2
// before any query runs.
func TestAuditCannotAnswer(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		setup     func(t *testing.T, base string)
		wantInErr string
	}{
		{"no audit workspace configured", "", func(*testing.T, string) {}, config.AuditWorkspaceKey},
		{"no drift observation", auditWorkspace, func(t *testing.T, base string) {
			seedBaseline(t, base, auditBaseline)
		}, "run 'azure-rd resource drift' first"},
		{"superseded observation", auditWorkspace, func(t *testing.T, base string) {
			tenantDir := seedBaseline(t, base, auditBaseline)
			seedObservation(t, tenantDir, auditBaseline)
			seedBaseline(t, base, "2026-09-29T00:00:00Z")
		}, "superseded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			tt.setup(t, base)
			err := runAudit(auditCommand(t, base, tt.workspace), nil)
			if code := cmdutil.ExitCode(err); code != auditExitCannotAnswer {
				t.Fatalf("exit code = %d (%v), want %d", code, err, auditExitCannotAnswer)
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q should mention %q", err, tt.wantInErr)
			}
			if _, statErr := os.Stat(drift.AuditPath(filepath.Join(base, auditTenant))); !os.IsNotExist(statErr) {
				t.Error("a refused run must not write audit.yaml")
			}
		})
	}
}

// TestNoTableQueried: the standalone command's own refusal — both tables
// failed — is detected from the artifact.
func TestNoTableQueried(t *testing.T) {
	a := &drift.Attribution{Tables: map[string]drift.TableStatus{
		drift.TableIntuneAuditLogs: {Status: drift.TableStatusFailed},
		drift.TableAuditLogs:       {Status: drift.TableStatusFailed},
	}}
	if !noTableQueried(a) {
		t.Error("noTableQueried() = false with both tables failed")
	}
	a.Tables[drift.TableAuditLogs] = drift.TableStatus{Status: drift.TableStatusOK}
	if noTableQueried(a) {
		t.Error("noTableQueried() = true with one table queried")
	}
}

// TestDriftRunAttributionMatchesTheObservation: the drift run attributes the
// observation exactly as written, so audit.yaml's observedAt and
// baselineGeneratedAt equal drift/metadata.yaml's byte for byte — the rule the
// browser and the analysis prompt gate on.
func TestDriftRunAttributionMatchesTheObservation(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	tenantDir := seedBaseline(t, t.TempDir(), auditBaseline)
	obs := seedObservation(t, tenantDir, auditBaseline)
	registry := handlers.NewRegistry(stubCredential{}, "", false)

	// Dry run: queries and reports, writes nothing.
	if err := writeDriftAttribution(context.Background(), emptyQuerier{}, registry, tenantDir, obs, auditWorkspace, audit.Selection{}, true); err != nil {
		t.Fatalf("dry run = %v", err)
	}
	if _, err := os.Stat(drift.AuditPath(tenantDir)); !os.IsNotExist(err) {
		t.Fatal("a dry run wrote audit.yaml")
	}

	if err := writeDriftAttribution(context.Background(), emptyQuerier{}, registry, tenantDir, obs, auditWorkspace, audit.Selection{}, false); err != nil {
		t.Fatalf("writeDriftAttribution() = %v", err)
	}
	metaRaw, err := os.ReadFile(drift.ObservationPath(tenantDir))
	if err != nil {
		t.Fatal(err)
	}
	auditRaw, err := os.ReadFile(drift.AuditPath(tenantDir))
	if err != nil {
		t.Fatal(err)
	}
	observedAt := regexp.MustCompile(`(?m)^observedAt: .*$`)
	if got, want := observedAt.Find(auditRaw), observedAt.Find(metaRaw); want == nil || string(got) != string(want) {
		t.Errorf("audit.yaml %q, metadata.yaml %q: observedAt must match byte for byte", got, want)
	}
	onDisk, err := drift.LoadObservation(tenantDir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := drift.LoadAttribution(tenantDir)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Matches(onDisk) || a.WorkspaceID != auditWorkspace {
		t.Errorf("attribution %+v does not match the observation on disk", a)
	}
	if got := a.Findings["Microsoft.Graph/groups/g.yaml"].Status; got != drift.AttributionNoEventInWindow {
		t.Errorf("finding status = %q, want no-event-in-window", got)
	}
}
