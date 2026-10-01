package runprep

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	betaodataerrors "github.com/microsoftgraph/msgraph-beta-sdk-go/models/odataerrors"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// intuneNestedMessage is the shape of an Intune refusal's OData message: an
// error object whose Message is itself a JSON string carrying the service
// version, the Activity ID and the URL — which here contain "403" and "429".
const intuneNestedMessage = `{"ErrorCode":"Forbidden","Message":"{\r\n  \"_version\": 3,\r\n  \"Message\": \"An error has occurred - Operation ID (for customer support): 00000000-0000-0000-0000-000000000000 - Activity ID: 4031429a-0000-0000-0000-000000000000 - Url: https://fef.amsua0403.manage.microsoft.com/DeviceConfiguration/429/deviceConfigurations\",\r\n  \"CustomApiErrorPhrase\": \"\",\r\n  \"RetryAfter\": null,\r\n  \"ErrorSourceService\": \"\",\r\n  \"HttpHeaders\": \"{}\"\r\n}","Target":null,"Details":null,"InnerError":null,"InstanceAnnotations":[],"TypeAnnotation":null}`

func v1ODataError(status int, code, message string) error {
	main := odataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&message)
	e := odataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(status)
	return e
}

func betaODataError(status int, code, message string) error {
	main := betaodataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&message)
	e := betaodataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(status)
	return e
}

func armError(status int, code string) error {
	return &azcore.ResponseError{
		StatusCode: status,
		ErrorCode:  code,
		RawResponse: &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Body:       http.NoBody,
			Request: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Scheme: "https", Host: "management.azure.com", Path: "/subscriptions/x/resources"},
			},
		},
	}
}

// stubHandler is a ResourceHandler whose probe and listing answer from
// fields and count their calls; it makes no request.
type stubHandler struct {
	typeName  string
	perms     []string
	hasProbe  bool
	probeErr  error
	listErr   error
	probes    atomic.Int32
	listCalls atomic.Int32
	// block, when set, makes the probe wait for the context.
	block bool
}

func (h *stubHandler) GetType() string { return h.typeName }
func (h *stubHandler) List(context.Context) ([]string, error) {
	h.listCalls.Add(1)
	return nil, h.listErr
}
func (h *stubHandler) Fetch(context.Context, string) (interface{}, error) { return nil, nil }
func (h *stubHandler) Transform(interface{}) (*models.TransformedResource, error) {
	return nil, errors.New("not used")
}
func (h *stubHandler) GetDocumentationPrompt() string  { return "" }
func (h *stubHandler) RequiresDedicatedApp() bool      { return true }
func (h *stubHandler) RequiredPermissions() []string   { return h.perms }
func (h *stubHandler) HasAccessProbe() bool            { return h.hasProbe }
func (h *stubHandler) ProbeAccess(ctx context.Context) error {
	h.probes.Add(1)
	if h.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return h.probeErr
}

// listOnlyHandler is a ResourceHandler with no probe and no declared
// permissions, the way an ARM handler is: the access check lists it.
type listOnlyHandler struct {
	typeName  string
	listErr   error
	listCalls atomic.Int32
}

func (h *listOnlyHandler) GetType() string { return h.typeName }
func (h *listOnlyHandler) List(context.Context) ([]string, error) {
	h.listCalls.Add(1)
	return nil, h.listErr
}
func (h *listOnlyHandler) Fetch(context.Context, string) (interface{}, error) { return nil, nil }
func (h *listOnlyHandler) Transform(interface{}) (*models.TransformedResource, error) {
	return nil, errors.New("not used")
}
func (h *listOnlyHandler) GetDocumentationPrompt() string { return "" }

func registryOf(hs ...models.ResourceHandler) *handlers.Registry {
	r := handlers.NewEmptyRegistry()
	for _, h := range hs {
		r.Register(h.GetType(), h)
	}
	return r
}

// captureLog redirects the default logger into a buffer at debug level for
// the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	level := logger.Default.GetLevel()
	logger.Default.SetOutput(&buf)
	logger.SetLogLevel("debug")
	t.Cleanup(func() {
		logger.Default.SetOutput(os.Stderr)
		logger.Default.SetLevel(level)
	})
	return &buf
}

func TestPermissionGroup(t *testing.T) {
	tests := map[string]string{
		"DeviceManagementConfiguration.Read.All":      "DeviceManagementConfiguration",
		"DeviceManagementConfiguration.ReadWrite.All": "DeviceManagementConfiguration",
		"Policy.Read.All":                             "Policy",
		"Policy.ReadWrite.ConditionalAccess":          "Policy.ReadWrite.ConditionalAccess",
		"Reader":                                      "Reader",
	}
	for perm, want := range tests {
		if got := PermissionGroup(perm); got != want {
			t.Errorf("PermissionGroup(%q) = %q, want %q", perm, got, want)
		}
	}
}

func ExamplePermissionGroup() {
	fmt.Println(PermissionGroup("DeviceManagementConfiguration.ReadWrite.All"))
	// Output: DeviceManagementConfiguration
}

func TestPlanProbes(t *testing.T) {
	types := []TypeInfo{
		{Type: "Microsoft.Graph/deviceConfigurations", Group: "DeviceManagementConfiguration", HasAccessProbe: true},
		{Type: "Microsoft.Graph/assignmentFilters", Group: "DeviceManagementConfiguration"},
		{Type: "Microsoft.Graph/namedLocations", Group: "Policy"},
		{Type: "Microsoft.Graph/conditionalAccessPolicies", Group: "Policy"},
		{Type: "Microsoft.Storage/storageAccounts", Group: AzureRBACGroup},
		{Type: "Microsoft.Resources/resourceGroups", Group: AzureRBACGroup},
		{Type: "Microsoft.Graph/unprobed"},
	}
	want := []Probe{
		{Group: AzureRBACGroup, ProbeType: "Microsoft.Resources/resourceGroups",
			Blocks: []string{"Microsoft.Resources/resourceGroups", "Microsoft.Storage/storageAccounts"}},
		{Group: "DeviceManagementConfiguration", ProbeType: "Microsoft.Graph/deviceConfigurations",
			Blocks: []string{"Microsoft.Graph/assignmentFilters", "Microsoft.Graph/deviceConfigurations"}},
		{Group: "Policy", ProbeType: "Microsoft.Graph/conditionalAccessPolicies",
			Blocks: []string{"Microsoft.Graph/conditionalAccessPolicies", "Microsoft.Graph/namedLocations"}},
	}
	if got := PlanProbes(types); !reflect.DeepEqual(got, want) {
		t.Errorf("PlanProbes() =\n%+v\nwant\n%+v", got, want)
	}
	if got := PlanProbes(nil); len(got) != 0 {
		t.Errorf("PlanProbes(nil) = %+v, want none", got)
	}
}

func TestPlanRunProbes(t *testing.T) {
	registry := registryOf(
		&stubHandler{typeName: "Microsoft.Graph/deviceConfigurations", perms: []string{"DeviceManagementConfiguration.ReadWrite.All"}, hasProbe: true},
		&stubHandler{typeName: "Microsoft.Graph/assignmentFilters", perms: []string{"DeviceManagementConfiguration.Read.All"}},
		&stubHandler{typeName: "Microsoft.Graph/organization", perms: nil},
		&listOnlyHandler{typeName: "Microsoft.Storage/storageAccounts"},
	)
	effective := []string{"Microsoft.Graph/assignmentFilters", "Microsoft.Graph/deviceConfigurations",
		"Microsoft.Graph/organization", "Microsoft.Storage/storageAccounts"}

	t.Run("with a subscription the ARM group is probed", func(t *testing.T) {
		got := PlanRunProbes(registry, effective, "sub-1", nil, "")
		want := []Probe{
			{Group: AzureRBACGroup, ProbeType: "Microsoft.Storage/storageAccounts", Blocks: []string{"Microsoft.Storage/storageAccounts"}},
			{Group: "DeviceManagementConfiguration", ProbeType: "Microsoft.Graph/deviceConfigurations",
				Blocks: []string{"Microsoft.Graph/assignmentFilters", "Microsoft.Graph/deviceConfigurations"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("PlanRunProbes() =\n%+v\nwant\n%+v", got, want)
		}
	})
	t.Run("without a subscription ARM types are dropped", func(t *testing.T) {
		got := PlanRunProbes(registry, effective, "", nil, "")
		if len(got) != 1 || got[0].Group != "DeviceManagementConfiguration" {
			t.Errorf("PlanRunProbes() = %+v, want only the Graph group", got)
		}
	})
	t.Run("a resource-id run plans nothing", func(t *testing.T) {
		if got := PlanRunProbes(registry, effective, "sub-1", []string{"/subscriptions/s/resourceGroups/rg"}, ""); got != nil {
			t.Errorf("PlanRunProbes() = %+v, want nil", got)
		}
	})
	t.Run("a resource-group run plans nothing", func(t *testing.T) {
		if got := PlanRunProbes(registry, effective, "sub-1", nil, "rg"); got != nil {
			t.Errorf("PlanRunProbes() = %+v, want nil", got)
		}
	})
}

func TestClassifyProbe(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ProbeOutcome
	}{
		{name: "nil", err: nil, want: ProbeAllowed},
		{name: "beta 401 with the nested Intune body", err: fmt.Errorf("failed to list device configurations: %w (hint: x)", betaODataError(http.StatusUnauthorized, "UnknownError", intuneNestedMessage)), want: ProbeRefused},
		{name: "v1.0 403 Authorization_RequestDenied", err: v1ODataError(http.StatusForbidden, "Authorization_RequestDenied", "Insufficient privileges to complete the operation."), want: ProbeRefused},
		{name: "ARM 403 AuthorizationFailed", err: fmt.Errorf("failed to list resources: %w", armError(http.StatusForbidden, "AuthorizationFailed")), want: ProbeRefused},
		{name: "404", err: v1ODataError(http.StatusNotFound, "Request_ResourceNotFound", "gone"), want: ProbeInconclusive},
		{name: "400", err: betaODataError(http.StatusBadRequest, "BadRequest", "bad $top"), want: ProbeInconclusive},
		{name: "429", err: v1ODataError(http.StatusTooManyRequests, "TooManyRequests", "slow down"), want: ProbeInconclusive},
		{name: "500", err: armError(http.StatusInternalServerError, "InternalServerError"), want: ProbeInconclusive},
		{name: "503", err: betaODataError(http.StatusServiceUnavailable, "ServiceUnavailable", "later"), want: ProbeInconclusive},
		{name: "authentication failed", err: fmt.Errorf("x: %w", &azidentity.AuthenticationFailedError{}), want: ProbeSignInFailed},
		{name: "authentication required", err: fmt.Errorf("device-code sign-in failed: %w", &azidentity.AuthenticationRequiredError{}), want: ProbeSignInFailed},
		{name: "credential unavailable", err: fmt.Errorf("x: %w", azidentity.NewCredentialUnavailableError("AzureCLICredential: please run 'az login'")), want: ProbeSignInFailed},
		{name: "credential unavailable in a joined error", err: errors.Join(errors.New("a"), azidentity.NewCredentialUnavailableError("b")), want: ProbeSignInFailed},
		{name: "probe deadline", err: fmt.Errorf("x: %w", context.DeadlineExceeded), want: ProbeInconclusive},
		{name: "cancelled", err: fmt.Errorf("x: %w", context.Canceled), want: ProbeCancelled},
		{name: "untyped text naming 403 and 429", err: errors.New("request 403 failed, retry after 429: Forbidden " + intuneNestedMessage), want: ProbeInconclusive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyProbe(tt.err); got != tt.want {
				t.Errorf("ClassifyProbe() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckAccess(t *testing.T) {
	t.Run("nothing planned does nothing", func(t *testing.T) {
		if err := CheckAccess(context.Background(), handlers.NewEmptyRegistry(), nil, "", 0, 1); err != nil {
			t.Fatalf("CheckAccess() = %v, want nil", err)
		}
	})

	t.Run("all allowed", func(t *testing.T) {
		a := &stubHandler{typeName: "t/a", hasProbe: true}
		b := &listOnlyHandler{typeName: "t/b"}
		probes := []Probe{{Group: "A", ProbeType: "t/a"}, {Group: AzureRBACGroup, ProbeType: "t/b"}}
		if err := CheckAccess(context.Background(), registryOf(a, b), probes, "sub", time.Minute, 2); err != nil {
			t.Fatalf("CheckAccess() = %v, want nil", err)
		}
		if a.probes.Load() != 1 || a.listCalls.Load() != 0 {
			t.Errorf("prober: %d probes, %d listings; want 1 probe and no listing", a.probes.Load(), a.listCalls.Load())
		}
		if b.listCalls.Load() != 1 {
			t.Errorf("a handler without a probe is probed by its listing; got %d listings", b.listCalls.Load())
		}
	})

	t.Run("one refused refuses the run, the others still complete", func(t *testing.T) {
		buf := captureLog(t)
		refused := &stubHandler{typeName: "t/intune", probeErr: fmt.Errorf("failed to list device configurations: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)",
			betaODataError(http.StatusUnauthorized, "UnknownError", intuneNestedMessage))}
		entra := &stubHandler{typeName: "t/policy", probeErr: v1ODataError(http.StatusForbidden, "Authorization_RequestDenied", "Requires one of the roles: Security Reader.\nmore")}
		ok := &stubHandler{typeName: "t/ok"}
		inconclusive := &stubHandler{typeName: "t/flaky", probeErr: v1ODataError(http.StatusServiceUnavailable, "ServiceUnavailable", "later")}
		probes := []Probe{
			{Group: "DeviceManagementConfiguration", ProbeType: "t/intune", Blocks: []string{"t/intune", "t/other"}},
			{Group: "Group", ProbeType: "t/ok", Blocks: []string{"t/ok"}},
			{Group: "Organization", ProbeType: "t/flaky", Blocks: []string{"t/flaky"}},
			{Group: "Policy", ProbeType: "t/policy", Blocks: []string{"t/policy"}},
		}
		err := CheckAccess(context.Background(), registryOf(refused, entra, ok, inconclusive), probes, "sub", time.Minute, 2)
		if !errors.Is(err, ErrAccessRefused) {
			t.Fatalf("CheckAccess() = %v, want ErrAccessRefused", err)
		}
		if !strings.Contains(err.Error(), "refused 2 of 4 permission groups") {
			t.Errorf("error %q does not count the refused groups", err)
		}
		for _, h := range []*stubHandler{refused, entra, ok, inconclusive} {
			if h.probes.Load() != 1 {
				t.Errorf("%s probed %d times, want 1", h.typeName, h.probes.Load())
			}
		}
		out := buf.String()
		intune := strings.Index(out, "group=DeviceManagementConfiguration")
		policy := strings.Index(out, "group=Policy")
		if intune < 0 || policy < 0 || intune > policy {
			t.Errorf("refusals not logged per group in group order:\n%s", out)
		}
		for _, want := range []string{"Access refused", "status=401", "code=Forbidden", "no Intune role for this account",
			"Requires one of the roles: Security Reader.", "Authorization_RequestDenied", "Access check inconclusive"} {
			if !strings.Contains(out, want) {
				t.Errorf("log lacks %q:\n%s", want, out)
			}
		}
	})

	t.Run("inconclusive does not refuse", func(t *testing.T) {
		h := &stubHandler{typeName: "t/x", probeErr: v1ODataError(http.StatusNotFound, "Request_ResourceNotFound", "gone")}
		if err := CheckAccess(context.Background(), registryOf(h), []Probe{{Group: "Organization", ProbeType: "t/x"}}, "", time.Minute, 1); err != nil {
			t.Fatalf("CheckAccess() = %v, want nil", err)
		}
	})

	t.Run("a probe timeout is inconclusive", func(t *testing.T) {
		h := &stubHandler{typeName: "t/slow", block: true}
		if err := CheckAccess(context.Background(), registryOf(h), []Probe{{Group: "Group", ProbeType: "t/slow"}}, "", time.Millisecond, 1); err != nil {
			t.Fatalf("CheckAccess() = %v, want nil", err)
		}
	})

	t.Run("sign-in failure and ARM refusal hints", func(t *testing.T) {
		buf := captureLog(t)
		signIn := &stubHandler{typeName: "t/g", probeErr: fmt.Errorf("device-code sign-in failed: %w", &azidentity.AuthenticationRequiredError{})}
		arm := &listOnlyHandler{typeName: "t/arm", listErr: armError(http.StatusForbidden, "AuthorizationFailed")}
		probes := []Probe{{Group: AzureRBACGroup, ProbeType: "t/arm"}, {Group: "Group", ProbeType: "t/g"}}
		if err := CheckAccess(context.Background(), registryOf(signIn, arm), probes, "sub-42", time.Minute, 2); !errors.Is(err, ErrAccessRefused) {
			t.Fatalf("CheckAccess() = %v, want ErrAccessRefused", err)
		}
		out := buf.String()
		for _, want := range []string{"no Azure RBAC Reader role on subscription sub-42", "status=\"sign-in failed\"", "sign-in failed: "} {
			if !strings.Contains(out, want) {
				t.Errorf("log lacks %q:\n%s", want, out)
			}
		}
	})

	t.Run("an interrupted run returns the context error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		h := &stubHandler{typeName: "t/x", block: true}
		err := CheckAccess(ctx, registryOf(h), []Probe{{Group: "Group", ProbeType: "t/x"}}, "", time.Minute, 1)
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrAccessRefused) {
			t.Fatalf("CheckAccess() = %v, want context.Canceled", err)
		}
	})
}

// TestCheckAccessConcurrencyBound proves the probes run concurrently but
// never more than the bound at a time.
func TestCheckAccessConcurrencyBound(t *testing.T) {
	var running, peak atomic.Int32
	var mu sync.Mutex
	gate := make(chan struct{})
	var hs []models.ResourceHandler
	var probes []Probe
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("t/%d", i)
		hs = append(hs, &boundedHandler{stubHandler: &stubHandler{typeName: name}, running: &running, peak: &peak, mu: &mu, gate: gate})
		probes = append(probes, Probe{Group: fmt.Sprintf("G%d", i), ProbeType: name})
	}
	done := make(chan error, 1)
	go func() { done <- CheckAccess(context.Background(), registryOf(hs...), probes, "", time.Minute, 2) }()
	close(gate)
	if err := <-done; err != nil {
		t.Fatalf("CheckAccess() = %v", err)
	}
	if peak.Load() > 2 {
		t.Errorf("peak concurrency %d exceeds the bound 2", peak.Load())
	}
}

type boundedHandler struct {
	*stubHandler
	running, peak *atomic.Int32
	mu            *sync.Mutex
	gate          chan struct{}
}

func (h *boundedHandler) ProbeAccess(context.Context) error {
	n := h.running.Add(1)
	h.mu.Lock()
	if n > h.peak.Load() {
		h.peak.Store(n)
	}
	h.mu.Unlock()
	<-h.gate
	h.running.Add(-1)
	return nil
}

func TestPreparedCheckAccess(t *testing.T) {
	refusing := &stubHandler{typeName: "Microsoft.Graph/groups", perms: []string{"Group.Read.All"}, hasProbe: true,
		probeErr: v1ODataError(http.StatusForbidden, "Authorization_RequestDenied", "denied")}

	t.Run("a refused group refuses the preparation", func(t *testing.T) {
		p := &Prepared{Registry: registryOf(refusing), EffectiveTypes: []string{"Microsoft.Graph/groups"},
			WorkerConfig: models.DefaultWorkerConfig(), Timeout: 30}
		if err := p.CheckAccess(context.Background()); !errors.Is(err, ErrAccessRefused) {
			t.Fatalf("CheckAccess() = %v, want ErrAccessRefused", err)
		}
	})

	t.Run("a resource-id run never probes", func(t *testing.T) {
		h := &stubHandler{typeName: "Microsoft.Graph/groups", perms: []string{"Group.Read.All"}, hasProbe: true, probeErr: refusing.probeErr}
		p := &Prepared{Registry: registryOf(h), EffectiveTypes: []string{"Microsoft.Graph/groups"},
			ResourceIDs: []string{"some-id"}, WorkerConfig: models.DefaultWorkerConfig()}
		if err := p.CheckAccess(context.Background()); err != nil {
			t.Fatalf("CheckAccess() = %v, want nil", err)
		}
		if h.probes.Load() != 0 || h.listCalls.Load() != 0 {
			t.Errorf("the prober was called for a --resource-id run")
		}
	})
}

// TestSecretResolutionLoggedOncePerPrepare follows Prepare's sequence — the
// offline registry, the real registry, then the run's own log line — and
// asserts the line appears exactly once, under the configuration key's name.
func TestSecretResolutionLoggedOncePerPrepare(t *testing.T) {
	buf := captureLog(t)
	cred, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		t.Fatal(err)
	}
	handlers.NewRegistry(cred, "", true)
	handlers.NewRegistry(cred, "", true)
	LogSecretResolution(true)

	out := buf.String()
	if n := strings.Count(out, "Secret resolution enabled"); n != 1 {
		t.Errorf("\"Secret resolution enabled\" logged %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "key=resolve-secrets") || strings.Contains(out, "--resolve-secrets") {
		t.Errorf("the line does not name the configuration key:\n%s", out)
	}

	buf.Reset()
	LogSecretResolution(false)
	if buf.Len() != 0 {
		t.Errorf("logged without resolve-secrets: %s", buf.String())
	}
}
