package drift

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// attributionState is what the analysis prompt knows about drift/audit.yaml:
// absent, unreadable, outdated (it attributes another observation) or current.
type attributionState struct {
	attr     *Attribution
	current  bool
	loadNote string
}

// loadAttributionState reads drift/audit.yaml and decides whether it belongs
// to obs. It never fails: attribution is enrichment.
func loadAttributionState(tenantDir string, obs *Observation) attributionState {
	a, err := LoadAttribution(tenantDir)
	switch {
	case errors.Is(err, ErrNoAttribution):
		return attributionState{}
	case err != nil:
		return attributionState{loadNote: err.Error()}
	}
	return attributionState{attr: a, current: a.Matches(*obs)}
}

// usable returns the attribution only when it describes this observation.
func (s attributionState) usable() *Attribution {
	if s.current {
		return s.attr
	}
	return nil
}

// renderAttributionSummary renders the observation block's attribution line:
// where the attribution came from, each table's status and the status counts —
// or why there is none.
func renderAttributionSummary(s attributionState) string {
	switch {
	case s.loadNote != "":
		return fmt.Sprintf("- Attribution: unreadable (%s) — no finding is attributed\n", s.loadNote)
	case s.attr == nil:
		return "- Attribution: none (no drift/audit.yaml — configure audit-workspace-id or run 'azure-rd resource audit')\n"
	case !s.current:
		return fmt.Sprintf("- Attribution: outdated (belongs to observation `%s`) — not used; run 'azure-rd resource audit' again\n", s.attr.ObservedAt)
	}
	a := s.attr
	c := a.Counts
	var b strings.Builder
	fmt.Fprintf(&b, "- Attribution: workspace `%s`, queried at `%s`; tables: %s\n", a.WorkspaceID, a.QueriedAt, tableCells(a.Tables))
	fmt.Fprintf(&b, "  - Findings: matched %d · no event in window %d · no join key %d · retention exceeded %d · query failed %d · not queried %d\n",
		c.Matched, c.NoEventInWindow, c.NoJoinKey, c.RetentionExceeded, c.QueryFailed, c.NotQueried)
	b.WriteString("  - Audit records arrive with an ingestion lag: a change made shortly before the observation may not be attributed yet, so \"no event in window\" is not proof that nobody changed the resource\n")
	return b.String()
}

// tableCells renders each table's status, sorted by table name.
func tableCells(tables map[string]TableStatus) string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	cells := make([]string, 0, len(names))
	for _, name := range names {
		ts := tables[name]
		switch {
		case ts.Status != TableStatusOK:
			cells = append(cells, fmt.Sprintf("`%s` %s (%s)", name, ts.Status, ts.Reason))
		case ts.Earliest == "":
			cells = append(cells, fmt.Sprintf("`%s` ok (no rows)", name))
		default:
			cells = append(cells, fmt.Sprintf("`%s` ok (earliest row `%s`)", name, ts.Earliest))
		}
	}
	if len(cells) == 0 {
		return "none recorded"
	}
	return strings.Join(cells, " · ")
}

// findingAttribution renders who changed one finding, newest first, or why
// that is unknown. Nothing is rendered without a current attribution.
func findingAttribution(key string, attr *Attribution) string {
	if attr == nil {
		return ""
	}
	af, ok := attr.Findings[key]
	if !ok {
		return "- Attribution unavailable (not recorded in drift/audit.yaml)\n"
	}
	if af.Status != AttributionMatched || len(af.Events) == 0 {
		if af.Reason == "" {
			return fmt.Sprintf("- Attribution unavailable (%s)\n", af.Status)
		}
		return fmt.Sprintf("- Attribution unavailable (%s: %s)\n", af.Status, af.Reason)
	}
	var b strings.Builder
	for _, ev := range af.Events {
		actor := ev.Actor
		if actor == "" {
			actor = "unknown actor"
		}
		fmt.Fprintf(&b, "- Changed by: %s (%s) at `%s` — %s, %s, correlation `%s`\n",
			actor, ev.ActorType, ev.At, ev.Activity, ev.Result, ev.CorrelationID)
	}
	return b.String()
}
