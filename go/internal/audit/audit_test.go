package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azlogs"
	"gopkg.in/yaml.v3"
)

const (
	intuneType  = "Microsoft.Graph/deviceConfigurations"
	entraType   = "Microsoft.Graph/conditionalAccessPolicies"
	appProtType = "Microsoft.Graph/iosManagedAppProtections"
	enrollType  = "Microsoft.Graph/deviceEnrollmentConfigurations"

	guidA = "11111111-2222-3333-4444-555555555555"
	guidB = "66666666-7777-8888-9999-000000000000"
	guidC = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	windowFrom = "2026-09-28T00:00:00Z"
	windowTo   = "2026-09-30T00:00:00Z"
)

// stubCredential is a no-network credential, so the real registry registers
// every Graph handler.
type stubCredential struct{}

func (stubCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "stub", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func testRegistry() *handlers.Registry {
	return handlers.NewRegistry(stubCredential{}, "", false)
}

// fakeQuerier answers the retention probe and the event queries per table
// from recorded rows, and records every query it was asked.
type fakeQuerier struct {
	earliest map[string]any
	probeErr map[string]error
	eventErr map[string]error
	rows     map[string][]Row
	queries  []string
	bounded  []bool
}

func newFake() *fakeQuerier {
	return &fakeQuerier{
		earliest: map[string]any{
			drift.TableIntuneAuditLogs: "2020-01-01T00:00:00.0000000Z",
			drift.TableAuditLogs:       "2020-01-01T00:00:00.0000000Z",
		},
		probeErr: map[string]error{},
		eventErr: map[string]error{},
		rows:     map[string][]Row{},
	}
}

func (f *fakeQuerier) Query(_ context.Context, _ string, kql string, from, to *time.Time) ([]Row, error) {
	f.queries = append(f.queries, kql)
	f.bounded = append(f.bounded, from != nil && to != nil)
	table := strings.SplitN(kql, "\n", 2)[0]
	if strings.Contains(kql, "summarize") {
		if from != nil || to != nil {
			return nil, errors.New("the retention probe must run without a timespan")
		}
		if err := f.probeErr[table]; err != nil {
			return nil, err
		}
		return []Row{{colEarliest: f.earliest[table]}}, nil
	}
	if from == nil || to == nil {
		return nil, errors.New("an event query must be bounded by the window")
	}
	if err := f.eventErr[table]; err != nil {
		return nil, err
	}
	return f.rows[table], nil
}

func (f *fakeQuerier) eventQueries(table string) []string {
	var out []string
	for _, q := range f.queries {
		if strings.HasPrefix(q, table+"\n") && !strings.Contains(q, "summarize") {
			out = append(out, q)
		}
	}
	return out
}

func observation(findings map[string]drift.Finding) drift.Observation {
	return drift.Observation{
		ObservedAt: windowTo,
		Tenant:     "contoso.example.com",
		Baseline:   drift.BaselineRef{GeneratedAt: windowFrom},
		Findings:   findings,
	}
}

func changed(id string) drift.Finding {
	return drift.Finding{Verdict: drift.VerdictChanged, ResourceID: id}
}

func intuneRow(at, target, upn, app, op, result, corr string) Row {
	props, _ := json.Marshal(map[string]any{
		"Actor":           map[string]any{"UPN": upn, "ApplicationName": app},
		"TargetObjectIds": []string{target},
	})
	return Row{colTimeGenerated: at, colTarget: target, "Properties": string(props),
		"OperationName": op, "ResultType": result, colCorrelationID: corr}
}

func attribute(q Querier, obs drift.Observation, sel Selection) *drift.Attribution {
	return Attribute(context.Background(), q, testRegistry(), obs, Options{
		WorkspaceID: "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b",
		ToolVersion: "test",
		Now:         time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		Selection:   sel,
	})
}

func loadFixture(t *testing.T, name string) []Row {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var res azlogs.QueryResults
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	rows, err := rowsFromResults(res)
	if err != nil {
		t.Fatalf("rowsFromResults: %v", err)
	}
	return rows
}

// TestRowMappingFromRecordedResponses pins both tables' column names and actor
// paths onto the one event shape, from responses in the query API's format.
func TestRowMappingFromRecordedResponses(t *testing.T) {
	tests := []struct {
		fixture string
		toEvent func(Row) drift.AttributionEvent
		want    []drift.AttributionEvent
	}{
		{
			fixture: "intuneauditlogs.json",
			toEvent: intuneEvent,
			want: []drift.AttributionEvent{
				{At: "2026-09-29T08:15:42Z", Actor: "admin@contoso.example.com", ActorType: "user", Activity: "Patch DeviceConfiguration", Result: "success", CorrelationID: "c0000000-0000-0000-0000-000000000001"},
				{At: "2026-09-29T09:00:00Z", Actor: "Graph automation", ActorType: "application", Activity: "Assign DeviceConfiguration", Result: "failure", CorrelationID: "c0000000-0000-0000-0000-000000000002"},
			},
		},
		{
			fixture: "auditlogs.json",
			toEvent: entraEvent,
			want: []drift.AttributionEvent{
				{At: "2026-09-29T10:30:05Z", Actor: "jane@contoso.example.com", ActorType: "user", Activity: "Update conditional access policy", Result: "success", CorrelationID: "c0000000-0000-0000-0000-000000000003"},
				{At: "2026-09-29T10:31:00Z", Actor: "Policy pipeline", ActorType: "application", Activity: "Update conditional access policy", Result: "unknown", CorrelationID: "c0000000-0000-0000-0000-000000000004"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			rows := loadFixture(t, tt.fixture)
			if len(rows) != len(tt.want) {
				t.Fatalf("rows = %d, want %d", len(rows), len(tt.want))
			}
			for i, row := range rows {
				if got := tt.toEvent(row); got != tt.want[i] {
					t.Errorf("row %d = %+v, want %+v", i, got, tt.want[i])
				}
			}
		})
	}
}

// TestActorPrecedenceAndResultNormalisation: a user wins over an application,
// neither is unknown; results collapse onto success | failure | unknown.
func TestActorPrecedenceAndResultNormalisation(t *testing.T) {
	actorCases := []struct{ upn, app, wantActor, wantType string }{
		{"u@x", "App", "u@x", "user"},
		{"", "App", "App", "application"},
		{"", "", "", "unknown"},
	}
	for _, c := range actorCases {
		actor, typ := pickActor(c.upn, c.app)
		if actor != c.wantActor || typ != c.wantType {
			t.Errorf("pickActor(%q, %q) = %q, %q; want %q, %q", c.upn, c.app, actor, typ, c.wantActor, c.wantType)
		}
	}
	for in, want := range map[string]string{"Success": "success", "failure": "failure", "FAILURE": "failure", "timeout": "unknown", "": "unknown"} {
		if got := normaliseResult(in); got != want {
			t.Errorf("normaliseResult(%q) = %q, want %q", in, got, want)
		}
	}
	// A row whose dynamic column is not JSON maps to an unknown actor rather
	// than failing.
	ev := entraEvent(Row{"InitiatedBy": "not json", colTimeGenerated: "bad"})
	if ev.ActorType != "unknown" || ev.At != "" {
		t.Errorf("entraEvent(garbage) = %+v, want unknown actor and empty time", ev)
	}
}

// TestPartialResultIsAnError: a response carrying an error beside its rows
// would read as "no event", so it must fail the query.
func TestPartialResultIsAnError(t *testing.T) {
	var res azlogs.QueryResults
	if err := json.Unmarshal([]byte(`{"tables":[{"name":"PrimaryResult","columns":[],"rows":[]}],"error":{"code":"PartialError","message":"truncated"}}`), &res); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if _, err := rowsFromResults(res); err == nil {
		t.Error("rowsFromResults accepted a partial result")
	}
	if _, err := rowsFromResults(azlogs.QueryResults{}); err == nil {
		t.Error("rowsFromResults accepted a response without a table")
	}
}

// TestRouteEveryRegisteredType routes every type the real registry registers:
// each lands on exactly one of the two tables, or is not queried because it is
// ARM; singletons land on no-join-key.
func TestRouteEveryRegisteredType(t *testing.T) {
	registry := testRegistry()
	rt := newRouter(registry)
	types := registry.GetAllTypes()
	if len(types) < 40 {
		t.Fatalf("registry has %d types, want the full default set", len(types))
	}
	for _, rtype := range types {
		table, status, reason := rt.route(rtype+"/x.yaml", changed(guidA))
		switch {
		case models.DetectAPIType(rtype) != models.APIMicrosoftGraph:
			if status != drift.AttributionNotQueried || table != "" || reason != reasonARM {
				t.Errorf("%s: route = %q/%q/%q, want not-queried ARM", rtype, table, status, reason)
			}
		case table != drift.TableIntuneAuditLogs && table != drift.TableAuditLogs:
			t.Errorf("%s: routed to table %q, want one of the two audit tables", rtype, table)
		case noJoinKeyTypes[rtype] != "":
			if status != drift.AttributionNoJoinKey {
				t.Errorf("%s: status %q, want no-join-key for a singleton/pseudo-id type", rtype, status)
			}
		case status != "":
			t.Errorf("%s: status %q (%s), want queried", rtype, status, reason)
		}
	}

	for rtype, want := range map[string]string{
		intuneType:                       drift.TableIntuneAuditLogs,
		appProtType:                      drift.TableIntuneAuditLogs,
		entraType:                        drift.TableAuditLogs,
		"Microsoft.Graph/groups":         drift.TableAuditLogs,
		"Microsoft.Graph/namedLocations": drift.TableAuditLogs,
	} {
		if got, _, _ := rt.route(rtype+"/x.yaml", changed(guidA)); got != want {
			t.Errorf("%s routed to %q, want %q", rtype, got, want)
		}
	}
}

// TestRouteJoinKeyGuard covers every way a finding cannot be joined, and the
// ids that embed a GUID and can.
func TestRouteJoinKeyGuard(t *testing.T) {
	registry := testRegistry()
	tests := []struct {
		name, key, id, wantStatus, wantReason string
	}{
		{"guid id", intuneType + "/a.yaml", guidA, "", ""},
		{"T_<guid> app protection", appProtType + "/a.yaml", "T_" + guidA, "", ""},
		{"<guid>_Suffix enrollment configuration", enrollType + "/a.yaml", guidA + "_DefaultPlatformRestrictions", "", ""},
		{"empty id", intuneType + "/a.yaml", "", drift.AttributionNoJoinKey, reasonNoResourceID},
		{"numeric role scope tag", "Microsoft.Graph/roleScopeTags/a.yaml", "0", drift.AttributionNoJoinKey, "numeric"},
		{"numeric id of a collection type", intuneType + "/a.yaml", "12345", drift.AttributionNoJoinKey, "contains no GUID"},
		{"quote in the id", intuneType + "/a.yaml", guidA + `"or 1==1`, drift.AttributionNoJoinKey, "contains no GUID"},
		{"singleton whose id is the tenant GUID", "Microsoft.Graph/organization/a.yaml", guidA, drift.AttributionNoJoinKey, "singleton"},
		{"unregistered Graph type", "Microsoft.Graph/unknownThings/a.yaml", guidA, drift.AttributionNotQueried, reasonUnregistered},
		{"ARM resource", "Microsoft.Storage/storageAccounts/a.yaml", "/subscriptions/s/resourceGroups/rg/providers/x", drift.AttributionNotQueried, reasonARM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, status, reason := Route(registry, tt.key, changed(tt.id))
			if status != tt.wantStatus || !strings.Contains(reason, tt.wantReason) {
				t.Errorf("Route() = %q (%q), want %q containing %q", status, reason, tt.wantStatus, tt.wantReason)
			}
		})
	}

	if got := joinIDs("T_" + strings.ToUpper(guidA)); len(got) != 2 || got[0] != "t_"+guidA || got[1] != guidA {
		t.Errorf("joinIDs(T_<GUID>) = %v, want the id and the embedded GUID, lower-cased", got)
	}
}

// TestNoGraphTypeLacksATable guards the routing's completeness: a Graph type
// whose permissions map to neither table is reported, not silently dropped.
func TestNoGraphTypeLacksATable(t *testing.T) {
	registry := handlers.NewEmptyRegistry()
	registry.Register("Microsoft.Graph/oddThings", &scopedHandler{perms: []string{"User.Read.All"}})
	_, status, reason := Route(registry, "Microsoft.Graph/oddThings/a.yaml", changed(guidA))
	if status != drift.AttributionNotQueried || reason != reasonNoTableMapped {
		t.Errorf("Route() = %q (%q), want not-queried %q", status, reason, reasonNoTableMapped)
	}
	registry.Register("Microsoft.Graph/mixed", &scopedHandler{perms: []string{"Group.Read.All", "DeviceManagementApps.Read.All"}})
	if table, _, _ := Route(registry, "Microsoft.Graph/mixed/a.yaml", changed(guidA)); table != drift.TableIntuneAuditLogs {
		t.Errorf("mixed permissions routed to %q, want IntuneAuditLogs to win", table)
	}
}

type scopedHandler struct {
	models.ResourceHandler
	perms []string
}

func (h *scopedHandler) RequiresDedicatedApp() bool    { return true }
func (h *scopedHandler) RequiredPermissions() []string { return h.perms }

// TestAttributeStatuses walks every status path through one observation.
func TestAttributeStatuses(t *testing.T) {
	f := newFake()
	f.rows[drift.TableIntuneAuditLogs] = []Row{
		intuneRow("2026-09-29T08:00:00.5Z", guidA, "old@x", "", "Patch", "Success", "c2"),
		intuneRow("2026-09-29T09:00:00Z", guidA, "new@x", "", "Patch", "Success", "c1"),
		// The same record reached twice (id and embedded GUID) is one event.
		intuneRow("2026-09-29T09:00:00Z", guidA, "new@x", "", "Patch", "Success", "c1"),
		// A row for no finding is ignored.
		intuneRow("2026-09-29T09:00:00Z", guidC, "x@x", "", "Patch", "Success", "c9"),
	}
	obs := observation(map[string]drift.Finding{
		intuneType + "/a.yaml":                     changed(guidA),
		intuneType + "/b.yaml":                     changed(guidB),
		entraType + "/c.yaml":                      changed(guidC),
		"Microsoft.Graph/organization/o.yaml":      changed(guidA),
		"Microsoft.Storage/storageAccounts/s.yaml": changed("/subscriptions/s/resourceGroups/rg/providers/x/y"),
	})

	a := attribute(f, obs, Selection{})

	matched := a.Findings[intuneType+"/a.yaml"]
	if matched.Status != drift.AttributionMatched || matched.Table != drift.TableIntuneAuditLogs || matched.Reason != "" {
		t.Fatalf("a = %+v, want matched in IntuneAuditLogs", matched)
	}
	if len(matched.Events) != 2 || matched.Events[0].Actor != "new@x" || matched.Events[1].At != "2026-09-29T08:00:00Z" {
		t.Errorf("events = %+v, want two, de-duplicated, newest first, whole seconds", matched.Events)
	}
	wantStatus := map[string]string{
		intuneType + "/b.yaml":                     drift.AttributionNoEventInWindow,
		entraType + "/c.yaml":                      drift.AttributionNoEventInWindow,
		"Microsoft.Graph/organization/o.yaml":      drift.AttributionNoJoinKey,
		"Microsoft.Storage/storageAccounts/s.yaml": drift.AttributionNotQueried,
	}
	for key, want := range wantStatus {
		if got := a.Findings[key]; got.Status != want || got.Events == nil || len(got.Events) != 0 {
			t.Errorf("%s = %+v, want %s with an empty event list", key, got, want)
		}
	}
	if a.Findings["Microsoft.Storage/storageAccounts/s.yaml"].Table != "" {
		t.Error("a not-queried finding must record no table")
	}
	if len(a.Findings) != len(obs.Findings) {
		t.Errorf("findings = %d, want exactly one per observation finding (%d)", len(a.Findings), len(obs.Findings))
	}
	want := drift.AttributionCounts{Matched: 1, NoEventInWindow: 2, NoJoinKey: 1, NotQueried: 1}
	if a.Counts != want {
		t.Errorf("counts = %+v, want %+v", a.Counts, want)
	}
	if a.Window.From != windowFrom || a.Window.To != windowTo || a.QueriedAt != "2026-09-30T12:00:00Z" {
		t.Errorf("header = %+v / %s, want the observation window and the injected time", a.Window, a.QueriedAt)
	}
	for _, table := range []string{drift.TableIntuneAuditLogs, drift.TableAuditLogs} {
		if ts := a.Tables[table]; ts.Status != drift.TableStatusOK || ts.Earliest != "2020-01-01T00:00:00Z" {
			t.Errorf("table %s = %+v, want ok with its earliest row", table, ts)
		}
	}
}

// TestAttributeRetention: a window starting before the table's earliest row,
// or a table without rows, makes "no event" unknown — never "unchanged".
func TestAttributeRetention(t *testing.T) {
	tests := []struct {
		name     string
		earliest any
		want     string
		reason   string
	}{
		{name: "window inside retention", earliest: "2026-09-01T00:00:00Z", want: drift.AttributionNoEventInWindow},
		{name: "window starts before the earliest row", earliest: "2026-09-29T00:00:00.123Z", want: drift.AttributionRetentionExceeded, reason: "2026-09-29T00:00:00Z"},
		{name: "table has no rows", earliest: nil, want: drift.AttributionRetentionExceeded, reason: "table has no rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.earliest[drift.TableIntuneAuditLogs] = tt.earliest
			f.rows[drift.TableIntuneAuditLogs] = []Row{intuneRow("2026-09-29T09:00:00Z", guidA, "u@x", "", "Patch", "Success", "c1")}
			a := attribute(f, observation(map[string]drift.Finding{
				intuneType + "/a.yaml": changed(guidA),
				intuneType + "/b.yaml": changed(guidB),
			}), Selection{})
			if got := a.Findings[intuneType+"/a.yaml"].Status; got != drift.AttributionMatched {
				t.Errorf("a finding with an event is %q, want matched regardless of retention", got)
			}
			got := a.Findings[intuneType+"/b.yaml"]
			if got.Status != tt.want || !strings.Contains(got.Reason, tt.reason) {
				t.Errorf("b = %+v, want %s with reason containing %q", got, tt.want, tt.reason)
			}
			if a.Tables[drift.TableIntuneAuditLogs].Status != drift.TableStatusOK {
				t.Errorf("retention never fails the table: %+v", a.Tables[drift.TableIntuneAuditLogs])
			}
		})
	}
}

// TestAttributeFailures: a permission failure names the grant, a token that
// cannot be minted fails both tables, and a failing event query fails its
// table — each turning that table's findings into query-failed.
func TestAttributeFailures(t *testing.T) {
	forbidden := &azcore.ResponseError{StatusCode: http.StatusForbidden, ErrorCode: "InsufficientAccessError"}
	findings := map[string]drift.Finding{
		intuneType + "/a.yaml":                changed(guidA),
		entraType + "/c.yaml":                 changed(guidC),
		"Microsoft.Graph/organization/o.yaml": changed(guidA),
	}

	t.Run("permission denied on one table", func(t *testing.T) {
		f := newFake()
		f.probeErr[drift.TableIntuneAuditLogs] = forbidden
		a := attribute(f, observation(findings), Selection{})
		ts := a.Tables[drift.TableIntuneAuditLogs]
		if ts.Status != drift.TableStatusFailed || !strings.Contains(ts.Reason, "Log Analytics Reader") || !strings.Contains(ts.Reason, "Data.Read") {
			t.Errorf("table = %+v, want failed naming the grant", ts)
		}
		if got := a.Findings[intuneType+"/a.yaml"]; got.Status != drift.AttributionQueryFailed || got.Reason != ts.Reason {
			t.Errorf("a = %+v, want query-failed with the table's reason", got)
		}
		if got := a.Findings[entraType+"/c.yaml"].Status; got != drift.AttributionNoEventInWindow {
			t.Errorf("the other table's finding = %q, want unaffected", got)
		}
		if got := a.Findings["Microsoft.Graph/organization/o.yaml"].Status; got != drift.AttributionNoJoinKey {
			t.Errorf("a no-join-key finding became %q; only routable findings fail", got)
		}
		if len(f.eventQueries(drift.TableIntuneAuditLogs)) != 0 {
			t.Error("a failed table must not be queried for events")
		}
	})

	t.Run("token cannot be minted", func(t *testing.T) {
		f := newFake()
		tokenErr := errors.New("DeviceCodeCredential: authentication required")
		f.probeErr[drift.TableIntuneAuditLogs] = tokenErr
		f.probeErr[drift.TableAuditLogs] = tokenErr
		a := attribute(f, observation(findings), Selection{})
		for _, table := range []string{drift.TableIntuneAuditLogs, drift.TableAuditLogs} {
			if a.Tables[table].Status != drift.TableStatusFailed {
				t.Errorf("%s = %+v, want failed", table, a.Tables[table])
			}
		}
		if a.Counts.QueryFailed != 2 || a.Counts.NoJoinKey != 1 {
			t.Errorf("counts = %+v, want both routable findings query-failed", a.Counts)
		}
	})

	t.Run("event query fails", func(t *testing.T) {
		f := newFake()
		f.eventErr[drift.TableAuditLogs] = errors.New("query timed out")
		a := attribute(f, observation(findings), Selection{})
		ts := a.Tables[drift.TableAuditLogs]
		if ts.Status != drift.TableStatusFailed || ts.Earliest != "2020-01-01T00:00:00Z" {
			t.Errorf("table = %+v, want failed keeping its earliest row", ts)
		}
		if got := a.Findings[entraType+"/c.yaml"].Status; got != drift.AttributionQueryFailed {
			t.Errorf("c = %q, want query-failed", got)
		}
	})
}

// TestAttributeChunksAndGuardsQueries: 201 ids take two queries, every id in a
// query passes the character-set guard, an embedded GUID is queried beside its
// id, and a row targeting either joins the finding.
func TestAttributeChunksAndGuardsQueries(t *testing.T) {
	findings := map[string]drift.Finding{}
	for i := 0; i < 201; i++ {
		findings[fmt.Sprintf("%s/r%03d.yaml", intuneType, i)] = changed(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	}
	f := newFake()
	a := attribute(f, observation(findings), Selection{})
	queries := f.eventQueries(drift.TableIntuneAuditLogs)
	if len(queries) != 2 {
		t.Fatalf("event queries = %d, want 2 for 201 ids", len(queries))
	}
	listPattern := regexp.MustCompile(`in~ \(([^)]*)\)`)
	idPattern := regexp.MustCompile(`^"[A-Za-z0-9_-]{1,128}"$`)
	total := 0
	for _, q := range queries {
		m := listPattern.FindStringSubmatch(q)
		if m == nil {
			t.Fatalf("query has no id list:\n%s", q)
		}
		for _, lit := range strings.Split(m[1], ", ") {
			if !idPattern.MatchString(lit) {
				t.Errorf("unsafe literal %q in query", lit)
			}
			total++
		}
	}
	if total != 201 {
		t.Errorf("ids queried = %d, want 201", total)
	}
	if len(f.eventQueries(drift.TableAuditLogs)) != 0 {
		t.Error("a table no finding routes to must not get event queries")
	}
	if a.Tables[drift.TableAuditLogs].Status != drift.TableStatusOK {
		t.Error("tables must carry both keys even when every finding routes to one table")
	}

	// Embedded GUIDs: queried as id plus GUID, joined by either.
	f = newFake()
	f.rows[drift.TableIntuneAuditLogs] = []Row{
		intuneRow("2026-09-29T09:00:00Z", guidA, "u@x", "", "Patch", "Success", "c1"),
		intuneRow("2026-09-29T09:00:00Z", strings.ToUpper(guidB)+"_DefaultPlatformRestrictions", "u@x", "", "Patch", "Success", "c2"),
	}
	a = attribute(f, observation(map[string]drift.Finding{
		appProtType + "/p.yaml": changed("T_" + guidA),
		enrollType + "/e.yaml":  changed(guidB + "_DefaultPlatformRestrictions"),
	}), Selection{})
	for _, key := range []string{appProtType + "/p.yaml", enrollType + "/e.yaml"} {
		if got := a.Findings[key].Status; got != drift.AttributionMatched {
			t.Errorf("%s = %q, want matched", key, got)
		}
	}
	q := f.eventQueries(drift.TableIntuneAuditLogs)[0]
	for _, id := range []string{`"t_` + guidA + `"`, `"` + guidA + `"`, `"` + guidB + `_defaultplatformrestrictions"`, `"` + guidB + `"`} {
		if !strings.Contains(q, id) {
			t.Errorf("query does not contain %s:\n%s", id, q)
		}
	}
}

// TestAttributeSelection: findings outside the selection are not-queried with
// the reason, so the file still names every finding.
func TestAttributeSelection(t *testing.T) {
	obs := observation(map[string]drift.Finding{
		intuneType + "/a.yaml": changed(guidA),
		entraType + "/c.yaml":  changed(guidC),
	})
	for name, sel := range map[string]Selection{
		"type":        {Types: []string{strings.ToLower(intuneType)}},
		"resource id": {ResourceIDs: []string{strings.ToUpper(guidA)}},
	} {
		t.Run(name, func(t *testing.T) {
			a := attribute(newFake(), obs, sel)
			if got := a.Findings[intuneType+"/a.yaml"].Status; got != drift.AttributionNoEventInWindow {
				t.Errorf("selected finding = %q, want queried", got)
			}
			got := a.Findings[entraType+"/c.yaml"]
			if got.Status != drift.AttributionNotQueried || got.Reason != reasonOutsideScope {
				t.Errorf("unselected finding = %+v, want not-queried %q", got, reasonOutsideScope)
			}
		})
	}
	a := attribute(newFake(), obs, Selection{ResourceGroup: "rg"})
	if a.Counts.NotQueried != 2 {
		t.Errorf("--resource-group counts = %+v, want every Graph finding not-queried", a.Counts)
	}
}

// TestAttributeBadWindow: an observation whose timestamps cannot bound a query
// fails its routable findings instead of querying an unbounded window.
func TestAttributeBadWindow(t *testing.T) {
	obs := observation(map[string]drift.Finding{intuneType + "/a.yaml": changed(guidA)})
	obs.ObservedAt = "not a time"
	f := newFake()
	a := attribute(f, obs, Selection{})
	if got := a.Findings[intuneType+"/a.yaml"]; got.Status != drift.AttributionQueryFailed || got.Reason != errBadWindow.Error() {
		t.Errorf("a = %+v, want query-failed for the bad window", got)
	}
	if len(f.eventQueries(drift.TableIntuneAuditLogs)) != 0 {
		t.Error("no event query may run without a window")
	}
}

// TestAttributeIsDeterministic: for a fixed query result and an injected
// time, the marshalled artifact is byte-identical across runs.
func TestAttributeIsDeterministic(t *testing.T) {
	build := func() []byte {
		f := newFake()
		f.rows[drift.TableIntuneAuditLogs] = []Row{
			intuneRow("2026-09-29T09:00:00Z", guidA, "b@x", "", "Patch", "Success", "c2"),
			intuneRow("2026-09-29T09:00:00Z", guidA, "a@x", "", "Patch", "Success", "c1"),
		}
		findings := map[string]drift.Finding{}
		for i := 0; i < 30; i++ {
			findings[fmt.Sprintf("%s/r%02d.yaml", intuneType, i)] = changed(guidA)
		}
		data, err := yaml.Marshal(attribute(f, observation(findings), Selection{}))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return data
	}
	first := build()
	for i := 0; i < 5; i++ {
		if got := build(); string(got) != string(first) {
			t.Fatalf("run %d differs:\n%s\nvs\n%s", i, got, first)
		}
	}
	if !strings.Contains(string(first), `correlationId: c1`) || strings.Index(string(first), "c1") > strings.Index(string(first), "c2") {
		t.Error("events with the same time must be ordered by correlation id")
	}
}

// ExampleAttribute shows an attribution of one Intune finding from a recorded
// audit row.
func ExampleAttribute() {
	f := newFake()
	f.rows[drift.TableIntuneAuditLogs] = []Row{intuneRow("2026-09-29T09:00:00Z", guidA, "admin@contoso.example.com", "", "Patch DeviceConfiguration", "Success", "c1")}
	a := attribute(f, observation(map[string]drift.Finding{intuneType + "/a.yaml": changed(guidA)}), Selection{})
	ev := a.Findings[intuneType+"/a.yaml"].Events[0]
	fmt.Println(a.Findings[intuneType+"/a.yaml"].Status, ev.Actor, ev.At)
	// Output: matched admin@contoso.example.com 2026-09-29T09:00:00Z
}
