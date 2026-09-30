package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"azure-resource-downloader/internal/drift"
)

// maxIDsPerQuery bounds the id list interpolated into one query, keeping every
// query well inside the service's query-length limit.
const maxIDsPerQuery = 200

// Column names every event query projects. Target is the matched target id;
// the rest are the table's own columns, pinned by the recorded fixtures.
const (
	colTimeGenerated = "TimeGenerated"
	colTarget        = "Target"
	colCorrelationID = "CorrelationId"
	colEarliest      = "Earliest"
)

// tableSpec is what differs between the two audit tables: the event query and
// the mapping of one of its rows onto the single event shape. Neither table's
// field names ever reach the artifact.
type tableSpec struct {
	name    string
	query   func(from, to string, idList string) string
	toEvent func(Row) drift.AttributionEvent
}

// tableSpecs lists the tables in a fixed order, so queries run
// deterministically.
var tableSpecs = []tableSpec{
	{name: drift.TableIntuneAuditLogs, query: intuneQuery, toEvent: intuneEvent},
	{name: drift.TableAuditLogs, query: entraQuery, toEvent: entraEvent},
}

// intuneQuery selects the IntuneAuditLogs rows whose target object ids include
// one of ids. Properties is a JSON string; TargetObjectIds is an array in it.
func intuneQuery(from, to, idList string) string {
	return fmt.Sprintf(`IntuneAuditLogs
| where TimeGenerated between (datetime(%s) .. datetime(%s))
| extend P = parse_json(Properties)
| mv-expand T = P.TargetObjectIds
| where tostring(T) in~ (%s)
| project TimeGenerated, Target = tostring(T), Properties, OperationName, ResultType, CorrelationId`, from, to, idList)
}

// entraQuery selects the AuditLogs rows whose target resources include one of
// ids.
func entraQuery(from, to, idList string) string {
	return fmt.Sprintf(`AuditLogs
| where TimeGenerated between (datetime(%s) .. datetime(%s))
| mv-expand T = TargetResources
| where tostring(T.id) in~ (%s)
| project TimeGenerated, Target = tostring(T.id), InitiatedBy, ActivityDisplayName, Result, CorrelationId`, from, to, idList)
}

// retentionQuery asks for a table's earliest row. It runs without a timespan,
// so the answer is the table's retention horizon, not the window's.
func retentionQuery(table string) string {
	return table + "\n| summarize " + colEarliest + " = min(" + colTimeGenerated + ")"
}

// kqlIDList renders ids as a quoted KQL list. Every id has passed the
// character-set guard in joinIDs; the check is repeated here so this function
// alone can never emit an unsafe literal.
func kqlIDList(ids []string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		if !joinIDPattern.MatchString(id) {
			continue
		}
		quoted = append(quoted, `"`+id+`"`)
	}
	return strings.Join(quoted, ", ")
}

// chunk splits ids into slices of at most size.
func chunk(ids []string, size int) [][]string {
	var out [][]string
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

// intuneEvent maps an IntuneAuditLogs row: actor from Properties.Actor.UPN
// (a user) else Properties.Actor.ApplicationName (an application), activity
// OperationName, result ResultType.
func intuneEvent(row Row) drift.AttributionEvent {
	props := asObject(row["Properties"])
	actor := asObject(props["Actor"])
	ev := drift.AttributionEvent{
		At:            formatTime(row[colTimeGenerated]),
		Activity:      asString(row["OperationName"]),
		Result:        normaliseResult(asString(row["ResultType"])),
		CorrelationID: asString(row[colCorrelationID]),
	}
	ev.Actor, ev.ActorType = pickActor(asString(actor["UPN"]), asString(actor["ApplicationName"]))
	return ev
}

// entraEvent maps an AuditLogs row: actor from
// InitiatedBy.user.userPrincipalName else InitiatedBy.app.displayName,
// activity ActivityDisplayName, result Result.
func entraEvent(row Row) drift.AttributionEvent {
	by := asObject(row["InitiatedBy"])
	ev := drift.AttributionEvent{
		At:            formatTime(row[colTimeGenerated]),
		Activity:      asString(row["ActivityDisplayName"]),
		Result:        normaliseResult(asString(row["Result"])),
		CorrelationID: asString(row[colCorrelationID]),
	}
	ev.Actor, ev.ActorType = pickActor(
		asString(asObject(by["user"])["userPrincipalName"]),
		asString(asObject(by["app"])["displayName"]))
	return ev
}

// pickActor prefers the user over the application; neither is unknown.
func pickActor(upn, app string) (string, string) {
	switch {
	case upn != "":
		return upn, "user"
	case app != "":
		return app, "application"
	default:
		return "", "unknown"
	}
}

// normaliseResult lower-cases a result onto success | failure | unknown.
func normaliseResult(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "success":
		return "success"
	case "failure":
		return "failure"
	default:
		return "unknown"
	}
}

// formatTime renders a TimeGenerated value as RFC3339 UTC with whole seconds
// (the fraction truncated), or "" when it is not a time.
func formatTime(v any) string {
	t, ok := parseTime(v)
	if !ok {
		return ""
	}
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// parseTime reads a datetime cell, which the query API returns as an RFC3339
// string with a fractional second.
func parseTime(v any) (time.Time, bool) {
	s := asString(v)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// asString renders a scalar cell as a string ("" for null or a non-scalar).
func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64, bool, json.Number:
		return fmt.Sprint(x)
	default:
		return ""
	}
}

// asObject reads a dynamic cell as an object. The query API returns dynamic
// columns as JSON text, and a JSON string column (IntuneAuditLogs.Properties)
// the same way, so both a string and an already-decoded map are accepted.
func asObject(v any) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		return x
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(x), &m); err != nil {
			return nil
		}
		return m
	default:
		return nil
	}
}
