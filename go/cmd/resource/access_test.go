package resource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/runprep"
	"azure-resource-downloader/internal/tenantdir"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func forbidden() error {
	code, msg := "Authorization_RequestDenied", "Insufficient privileges to complete the operation."
	main := odataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&msg)
	e := odataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(http.StatusForbidden)
	return e
}

// accessStub is a handler whose probe answers probeErr and whose listing
// returns one id; both count their calls and make no request.
type accessStub struct {
	typeName string
	perm     string
	probeErr error
	probed   atomic.Int32
	listed   atomic.Int32
}

func (h *accessStub) GetType() string { return h.typeName }
func (h *accessStub) List(context.Context) ([]string, error) {
	h.listed.Add(1)
	return []string{h.typeName + "/id-1"}, nil
}
func (h *accessStub) Fetch(context.Context, string) (interface{}, error) { return nil, nil }
func (h *accessStub) Transform(interface{}) (*models.TransformedResource, error) {
	return nil, errors.New("not used")
}
func (h *accessStub) GetDocumentationPrompt() string { return "" }
func (h *accessStub) RequiresDedicatedApp() bool     { return true }
func (h *accessStub) RequiredPermissions() []string  { return []string{h.perm} }
func (h *accessStub) HasAccessProbe() bool           { return true }
func (h *accessStub) ProbeAccess(context.Context) error {
	h.probed.Add(1)
	return h.probeErr
}

func TestListTenantAccessCheck(t *testing.T) {
	newRegistry := func() (*handlers.Registry, *accessStub, *accessStub) {
		groups := &accessStub{typeName: "Microsoft.Graph/groups", perm: "Group.Read.All"}
		policies := &accessStub{typeName: "Microsoft.Graph/namedLocations", perm: "Policy.Read.All", probeErr: forbidden()}
		r := handlers.NewEmptyRegistry()
		r.Register(groups.typeName, groups)
		r.Register(policies.typeName, policies)
		return r, groups, policies
	}
	types := []string{"Microsoft.Graph/groups", "Microsoft.Graph/namedLocations"}

	t.Run("a refused group lists nothing and exits 1", func(t *testing.T) {
		r, groups, policies := newRegistry()
		requests, _, _, err := listTenant(context.Background(), r, listScope{types: types, concurrency: 2})
		if !errors.Is(err, runprep.ErrAccessRefused) {
			t.Fatalf("listTenant() = %v, want ErrAccessRefused", err)
		}
		if code := cmdutil.ExitCode(err); code != 1 {
			t.Errorf("exit code %d, want 1", code)
		}
		if len(requests) != 0 || groups.listed.Load() != 0 || policies.listed.Load() != 0 {
			t.Errorf("listed after a refused access check (%d requests)", len(requests))
		}
		if groups.probed.Load() != 1 || policies.probed.Load() != 1 {
			t.Errorf("each group is probed once; got %d and %d", groups.probed.Load(), policies.probed.Load())
		}
	})

	t.Run("all clear lists as before", func(t *testing.T) {
		r, groups, _ := newRegistry()
		requests, _, _, err := listTenant(context.Background(), r, listScope{types: types[:1], concurrency: 1})
		if err != nil {
			t.Fatalf("listTenant() = %v", err)
		}
		if len(requests) != 1 || groups.listed.Load() != 1 {
			t.Errorf("got %d requests after %d listings, want 1 and 1", len(requests), groups.listed.Load())
		}
	})

	t.Run("a resource-id run is never probed", func(t *testing.T) {
		r, groups, policies := newRegistry()
		requests, _, _, err := listTenant(context.Background(), r, listScope{types: types, resourceIDs: []string{"some-id"}, concurrency: 1})
		if err != nil {
			t.Fatalf("listTenant() = %v", err)
		}
		if len(requests) != 1 || groups.probed.Load()+policies.probed.Load() != 0 {
			t.Errorf("got %d requests, %d probes; want 1 request and no probe", len(requests), groups.probed.Load()+policies.probed.Load())
		}
	})
}

// TestAccessRefusedRunWritesNothing drives download and drift through their
// RunE with a preparation whose access check refused: nothing may appear under
// the output directory, dry-run or not, and the exit codes are download 1,
// drift 2 ("cannot answer").
func TestAccessRefusedRunWritesNothing(t *testing.T) {
	refused := fmt.Errorf("%w 1 of 2 permission groups; narrow the run", runprep.ErrAccessRefused)
	orig := prepareRun
	t.Cleanup(func() { prepareRun = orig })
	prepareRun = func(context.Context, runprep.Options) (*runprep.Prepared, error) { return nil, refused }

	commands := []struct {
		name     string
		newCmd   func() *cobra.Command
		run      func(*cobra.Command, []string) error
		wantCode int
	}{
		{name: "download", newCmd: NewDownloadCommand, run: runDownload, wantCode: 1},
		{name: "drift", newCmd: NewDriftCommand, run: runDrift, wantCode: 2},
	}
	for _, c := range commands {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dry-run=%v", c.name, dryRun), func(t *testing.T) {
				output := t.TempDir()
				viper.Reset()
				t.Cleanup(viper.Reset)
				viper.Set("output", output)
				viper.Set("dry-run", dryRun)

				cmd := c.newCmd()
				cmd.SetContext(context.Background())
				err := c.run(cmd, nil)
				if !errors.Is(err, runprep.ErrAccessRefused) {
					t.Fatalf("run = %v, want ErrAccessRefused", err)
				}
				if code := cmdutil.ExitCode(err); code != c.wantCode {
					t.Errorf("exit code %d, want %d", code, c.wantCode)
				}
				entries, readErr := os.ReadDir(output)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if len(entries) != 0 {
					t.Errorf("a refused run wrote %d entries under the output directory", len(entries))
				}
			})
		}
	}
}

func TestDriftPrepareExit(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "access refused", err: fmt.Errorf("x: %w", runprep.ErrAccessRefused), want: 2},
		{name: "tenant mismatch", err: tenantdir.ErrMismatch, want: 2},
		{name: "tenant unresolved", err: tenantdir.ErrUnresolved, want: 2},
		{name: "anything else", err: errors.New("failed to create Azure client"), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cmdutil.ExitCode(prepareExit(tt.err)); got != tt.want {
				t.Errorf("exit code %d, want %d", got, tt.want)
			}
		})
	}
}
