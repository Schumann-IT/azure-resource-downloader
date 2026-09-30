// Package audit attributes each finding of a drift observation to an actor and
// a time, from the tenant's Log Analytics audit tables (IntuneAuditLogs for
// Intune types, AuditLogs for Entra ID types). The join key is the finding's
// resource id — the object GUID the tables record as the target — and the
// window is the observation's own interval, from the baseline's generatedAt to
// the observation's observedAt.
//
// Attribution is enrichment: it never changes a verdict, never fails a run,
// and records facts only. Where the join cannot be made it records why, with a
// status distinct from "no event found", because "could not look" and "nobody
// changed it" must never read the same.
//
// It is imported by cmd/resource only; the drift package owns the artifact's
// shape (drift.Attribution) so docs analyze-drift can read it without
// depending on this package.
package audit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/handlers"
)

// grantHint names the grant a permission failure needs.
const grantHint = "grant Log Analytics Reader on the workspace; a dedicated app also needs the Log Analytics API delegated permission Data.Read"

// errBadWindow is recorded when the observation's timestamps cannot bound a
// query.
var errBadWindow = errors.New("the observation window is not a valid RFC3339 interval")

// Selection is the run's selection; findings outside it are recorded as
// not-queried so the artifact still names every finding.
type Selection struct {
	Types         []string
	ResourceIDs   []string
	ResourceGroup string
}

// Options configures one attribution.
type Options struct {
	// WorkspaceID is the Log Analytics workspace id (GUID).
	WorkspaceID string
	// ToolVersion is recorded in the artifact.
	ToolVersion string
	// Now is the query time recorded as queriedAt; zero means time.Now().
	// Injected so tests produce deterministic bytes.
	Now time.Time
	// Selection narrows the findings that are queried.
	Selection Selection
}

// Attribute queries the audit tables for every finding of obs and returns the
// attribution: exactly one entry per observation finding, both tables' status,
// and the status counts. It never returns an error — every failure is recorded
// in the artifact (a failed table, query-failed findings) for the caller to
// report.
func Attribute(ctx context.Context, q Querier, registry *handlers.Registry, obs drift.Observation, opts Options) *drift.Attribution {
	a := newAttribution(obs, opts)
	pending := classify(newRouter(registry), obs, opts.Selection, a)
	from, to, werr := observationWindow(obs)

	for _, spec := range tableSpecs {
		ts := probeTable(ctx, q, opts.WorkspaceID, spec.name)
		keys := pending[spec.name]
		if ts.Status == drift.TableStatusOK && len(keys) > 0 {
			ts = resolveTable(ctx, q, opts.WorkspaceID, spec, keys, obs, from, to, werr, ts, a)
		} else {
			markFailed(a, keys, spec.name, ts.Reason)
		}
		a.Tables[spec.name] = ts
	}

	a.Tally()
	return a
}

// newAttribution builds the artifact's header from the observation.
func newAttribution(obs drift.Observation, opts Options) *drift.Attribution {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	return &drift.Attribution{
		Version:             drift.AttributionVersion,
		ObservedAt:          obs.ObservedAt,
		BaselineGeneratedAt: obs.Baseline.GeneratedAt,
		Tenant:              obs.Tenant,
		ToolVersion:         opts.ToolVersion,
		QueriedAt:           now.UTC().Format(time.RFC3339),
		WorkspaceID:         opts.WorkspaceID,
		Window:              drift.AttributionWindow{From: normaliseStamp(obs.Baseline.GeneratedAt), To: normaliseStamp(obs.ObservedAt)},
		Tables:              map[string]drift.TableStatus{},
		Findings:            map[string]drift.AttributionFinding{},
	}
}

// classify records the final status of every finding that is not queried and
// returns the rest, grouped by table.
func classify(rt *router, obs drift.Observation, sel Selection, a *drift.Attribution) map[string][]string {
	pending := map[string][]string{}
	for key, f := range obs.Findings {
		if !sel.contains(key, f) {
			a.Findings[key] = finding(drift.AttributionNotQueried, "", reasonOutsideScope)
			continue
		}
		table, status, reason := rt.route(key, f)
		if status != "" {
			if status == drift.AttributionNotQueried {
				table = ""
			}
			a.Findings[key] = finding(status, table, reason)
			continue
		}
		pending[table] = append(pending[table], key)
	}
	for table := range pending {
		sort.Strings(pending[table])
	}
	return pending
}

// probeTable asks a table for its earliest row. It runs for both tables
// whether or not a finding routes to them, so a missing diagnostic setting
// always surfaces as a failed table.
func probeTable(ctx context.Context, q Querier, workspaceID, table string) drift.TableStatus {
	rows, err := q.Query(ctx, workspaceID, retentionQuery(table), nil, nil)
	if err != nil {
		return drift.TableStatus{Status: drift.TableStatusFailed, Reason: failReason(err)}
	}
	ts := drift.TableStatus{Status: drift.TableStatusOK}
	if len(rows) > 0 {
		ts.Earliest = formatTime(rows[0][colEarliest])
	}
	return ts
}

// resolveTable runs the event queries for one table and records the status of
// every finding routed to it. It returns the table status, turned failed when
// an event query fails.
func resolveTable(ctx context.Context, q Querier, workspaceID string, spec tableSpec, keys []string,
	obs drift.Observation, from, to time.Time, werr error, ts drift.TableStatus, a *drift.Attribution) drift.TableStatus {
	if werr != nil {
		markFailed(a, keys, spec.name, werr.Error())
		return ts
	}

	byID := map[string][]string{}
	var ids []string
	for _, key := range keys {
		for _, id := range joinIDs(obs.Findings[key].ResourceID) {
			if _, seen := byID[id]; !seen {
				ids = append(ids, id)
			}
			byID[id] = append(byID[id], key)
		}
	}
	sort.Strings(ids)

	events, err := queryEvents(ctx, q, workspaceID, spec, ids, byID, from, to)
	if err != nil {
		ts = drift.TableStatus{Status: drift.TableStatusFailed, Reason: failReason(err), Earliest: ts.Earliest}
		markFailed(a, keys, spec.name, ts.Reason)
		return ts
	}

	absent, absentReason := absenceStatus(ts.Earliest, from)
	for _, key := range keys {
		evs := events[key]
		if len(evs) == 0 {
			a.Findings[key] = finding(absent, spec.name, absentReason)
			continue
		}
		sortEvents(evs)
		a.Findings[key] = drift.AttributionFinding{Status: drift.AttributionMatched, Table: spec.name, Reason: "", Events: evs}
	}
	return ts
}

// queryEvents runs the chunked event queries and returns the events per
// finding key, de-duplicated (an id and its embedded GUID may both match one
// record).
func queryEvents(ctx context.Context, q Querier, workspaceID string, spec tableSpec, ids []string,
	byID map[string][]string, from, to time.Time) (map[string][]drift.AttributionEvent, error) {
	fromS, toS := from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)
	events := map[string][]drift.AttributionEvent{}
	seen := map[string]map[drift.AttributionEvent]bool{}
	for _, part := range chunk(ids, maxIDsPerQuery) {
		rows, err := q.Query(ctx, workspaceID, spec.query(fromS, toS, kqlIDList(part)), &from, &to)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ev := spec.toEvent(row)
			for _, key := range byID[strings.ToLower(asString(row[colTarget]))] {
				if seen[key] == nil {
					seen[key] = map[drift.AttributionEvent]bool{}
				}
				if seen[key][ev] {
					continue
				}
				seen[key][ev] = true
				events[key] = append(events[key], ev)
			}
		}
	}
	return events, nil
}

// absenceStatus decides what "no event" means for a table: nothing changed in
// a window the table fully covers, or unknown when the window starts before
// the table's earliest row (or the table holds no rows at all).
func absenceStatus(earliest string, from time.Time) (string, string) {
	if earliest == "" {
		return drift.AttributionRetentionExceeded, "table has no rows"
	}
	e, err := time.Parse(time.RFC3339, earliest)
	if err == nil && from.Before(e) {
		return drift.AttributionRetentionExceeded, fmt.Sprintf("the window starts at %s, before the table's earliest row at %s",
			from.UTC().Format(time.RFC3339), earliest)
	}
	return drift.AttributionNoEventInWindow, ""
}

// markFailed records query-failed for every key routed to a table.
func markFailed(a *drift.Attribution, keys []string, table, reason string) {
	for _, key := range keys {
		a.Findings[key] = finding(drift.AttributionQueryFailed, table, reason)
	}
}

// failReason renders a query failure; a permission failure names the grant.
func failReason(err error) string {
	summary := azure.ErrorSummary(err)
	if azure.IsPermissionError(err) {
		return fmt.Sprintf("permission denied (%s): %s", summary, grantHint)
	}
	return summary
}

// finding builds an attribution entry without events.
func finding(status, table, reason string) drift.AttributionFinding {
	return drift.AttributionFinding{Status: status, Table: table, Reason: reason, Events: []drift.AttributionEvent{}}
}

// sortEvents orders events newest first, then by correlation id, then by the
// remaining fields so the order never depends on the service's row order.
func sortEvents(evs []drift.AttributionEvent) {
	sort.Slice(evs, func(i, j int) bool {
		a, b := evs[i], evs[j]
		if a.At != b.At {
			return a.At > b.At
		}
		if a.CorrelationID != b.CorrelationID {
			return a.CorrelationID < b.CorrelationID
		}
		if a.Activity != b.Activity {
			return a.Activity < b.Activity
		}
		return a.Actor < b.Actor
	})
}

// observationWindow parses the observation's interval.
func observationWindow(obs drift.Observation) (time.Time, time.Time, error) {
	from, ferr := time.Parse(time.RFC3339, obs.Baseline.GeneratedAt)
	to, terr := time.Parse(time.RFC3339, obs.ObservedAt)
	if ferr != nil || terr != nil || to.Before(from) {
		return time.Time{}, time.Time{}, errBadWindow
	}
	return from.UTC(), to.UTC(), nil
}

// normaliseStamp renders a timestamp as RFC3339 UTC with whole seconds, or
// returns it unchanged when it does not parse.
func normaliseStamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// contains reports whether a finding lies inside the run's selection.
func (s Selection) contains(key string, f drift.Finding) bool {
	if len(s.Types) > 0 && !containsFold(s.Types, drift.TypeOfKey(key)) {
		return false
	}
	if len(s.ResourceIDs) > 0 && !containsFold(s.ResourceIDs, f.ResourceID) {
		return false
	}
	if s.ResourceGroup != "" {
		marker := "/resourcegroups/" + strings.ToLower(s.ResourceGroup)
		id := strings.ToLower(f.ResourceID)
		if !strings.Contains(id, marker+"/") && !strings.HasSuffix(id, marker) {
			return false
		}
	}
	return true
}

// containsFold reports whether list holds s, case-insensitively.
func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}
