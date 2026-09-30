package drift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"azure-resource-downloader/internal/docs"

	"gopkg.in/yaml.v3"
)

// AuditFileName is the attribution artifact at the drift-tree root, beside the
// observation metadata and analyze.md. It is deliberately a separate file: the
// observation is computed from bytes the drift run fetched and stays
// deterministic, while attribution is copied from an external system (Log
// Analytics) with its own ingestion lag, retention and completeness, and must
// be allowed to be absent or partial without invalidating the observation.
// Payloads are always at least two levels deep, so it can never collide with
// one, and ClearTree sweeps it with the observation it belongs to.
const AuditFileName = "audit.yaml"

// AttributionVersion is the schema version written into audit.yaml.
const AttributionVersion = 1

// Per-finding attribution statuses. "No event found" and "could not look" are
// always distinct statuses, never the same rendering.
const (
	// AttributionMatched: at least one audit event targets the resource within
	// the observation window.
	AttributionMatched = "matched"
	// AttributionNoEventInWindow: the table was queried successfully, covers the
	// whole window, and holds no event for the resource.
	AttributionNoEventInWindow = "no-event-in-window"
	// AttributionNoJoinKey: the resource has no id the audit tables record as a
	// target (singletons, pseudo-ids, non-GUID ids).
	AttributionNoJoinKey = "no-join-key"
	// AttributionRetentionExceeded: the window starts before the table's
	// earliest row, so the absence of an event is unknown, not "unchanged".
	AttributionRetentionExceeded = "retention-exceeded"
	// AttributionQueryFailed: the table the finding routes to could not be
	// queried (no grant, no token, service error).
	AttributionQueryFailed = "query-failed"
	// AttributionNotQueried: no table is consulted for this finding (ARM type,
	// type not registered, no table mapped, outside the run's selection).
	AttributionNotQueried = "not-queried"
)

// Audit tables consulted, and the status each can carry.
const (
	// TableIntuneAuditLogs is the Log Analytics table Intune's diagnostic
	// settings ship to.
	TableIntuneAuditLogs = "IntuneAuditLogs"
	// TableAuditLogs is the Log Analytics table Entra ID's diagnostic settings
	// ship to.
	TableAuditLogs = "AuditLogs"
	// TableStatusOK marks a table whose retention probe succeeded.
	TableStatusOK = "ok"
	// TableStatusFailed marks a table that could not be queried.
	TableStatusFailed = "failed"
)

// ErrNoAttribution is returned by LoadAttribution when the tenant has no
// drift/audit.yaml — the normal state when no audit workspace is configured.
var ErrNoAttribution = errors.New("no drift attribution in export directory")

// Attribution is the on-disk shape of drift/audit.yaml: who changed each
// finding of one observation, and when, as recorded by the tenant's audit
// tables. Facts only — never a judgment such as "authorized" or "expected".
// Apart from QueriedAt it is deterministic for a fixed query result.
type Attribution struct {
	Version int `yaml:"version"`
	// ObservedAt and BaselineGeneratedAt are copied from the observation this
	// file attributes; a consumer trusts the file only while both equal the
	// observation's (see Matches).
	ObservedAt          string            `yaml:"observedAt"`
	BaselineGeneratedAt string            `yaml:"baselineGeneratedAt"`
	Tenant              string            `yaml:"tenant"`
	ToolVersion         string            `yaml:"toolVersion"`
	QueriedAt           string            `yaml:"queriedAt"`
	WorkspaceID         string            `yaml:"workspaceId"`
	Window              AttributionWindow `yaml:"window"`
	// Tables always carries both IntuneAuditLogs and AuditLogs.
	Tables   map[string]TableStatus        `yaml:"tables"`
	Counts   AttributionCounts             `yaml:"counts"`
	Findings map[string]AttributionFinding `yaml:"findings"`
}

// AttributionWindow is the interval queried: from the baseline's generatedAt to
// the observation's observedAt — exactly the interval the verdicts claim the
// change fell in.
type AttributionWindow struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// TableStatus records whether one audit table could be queried and the
// earliest row it holds ("" when unknown or the table is empty).
type TableStatus struct {
	Status   string `yaml:"status"`
	Reason   string `yaml:"reason"`
	Earliest string `yaml:"earliest"`
}

// AttributionCounts tallies the per-finding statuses.
type AttributionCounts struct {
	Matched           int `yaml:"matched"`
	NoEventInWindow   int `yaml:"noEventInWindow"`
	NoJoinKey         int `yaml:"noJoinKey"`
	RetentionExceeded int `yaml:"retentionExceeded"`
	QueryFailed       int `yaml:"queryFailed"`
	NotQueried        int `yaml:"notQueried"`
}

// AttributionFinding is the attribution of one observation finding, keyed
// exactly like the observation's findings. Reason is empty for matched and
// no-event-in-window; Events is non-empty for matched only.
type AttributionFinding struct {
	Status string             `yaml:"status"`
	Table  string             `yaml:"table"`
	Reason string             `yaml:"reason"`
	Events []AttributionEvent `yaml:"events"`
}

// AttributionEvent is one audit record targeting a finding's resource, mapped
// from either table onto one shape so no table's field names leak into the
// artifact.
type AttributionEvent struct {
	// At is the event time, RFC3339 UTC with whole seconds.
	At string `yaml:"at"`
	// Actor is the user principal name, or the application display name.
	Actor string `yaml:"actor"`
	// ActorType is user, application or unknown.
	ActorType string `yaml:"actorType"`
	// Activity is the audited operation.
	Activity string `yaml:"activity"`
	// Result is success, failure or unknown.
	Result        string `yaml:"result"`
	CorrelationID string `yaml:"correlationId"`
}

// Tally recomputes Counts from Findings.
func (a *Attribution) Tally() {
	a.Counts = AttributionCounts{}
	for _, f := range a.Findings {
		switch f.Status {
		case AttributionMatched:
			a.Counts.Matched++
		case AttributionNoEventInWindow:
			a.Counts.NoEventInWindow++
		case AttributionNoJoinKey:
			a.Counts.NoJoinKey++
		case AttributionRetentionExceeded:
			a.Counts.RetentionExceeded++
		case AttributionQueryFailed:
			a.Counts.QueryFailed++
		case AttributionNotQueried:
			a.Counts.NotQueried++
		}
	}
}

// Matches reports whether this attribution belongs to obs: both the
// observation time and the baseline it was decided against must be equal. An
// attribution of an earlier observation must never be read as describing the
// current one.
func (a *Attribution) Matches(obs Observation) bool {
	return a.ObservedAt == obs.ObservedAt && a.BaselineGeneratedAt == obs.Baseline.GeneratedAt
}

// AuditPath returns the path of a tenant's drift attribution.
func AuditPath(tenantDir string) string {
	return filepath.Join(tenantDir, DriftDirName, AuditFileName)
}

// LoadAttribution reads a tenant's drift/audit.yaml. It returns
// ErrNoAttribution when there is none, so callers can tell "attribution off or
// not run" from a read failure.
func LoadAttribution(tenantDir string) (*Attribution, error) {
	p := AuditPath(tenantDir)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNoAttribution, p)
		}
		return nil, fmt.Errorf("failed to read drift attribution: %w", err)
	}
	var a Attribution
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("failed to parse drift attribution: %w", err)
	}
	return &a, nil
}

// WriteAttribution persists a drift attribution at drift/audit.yaml,
// atomically. It never clears anything and never creates the drift/ tree: an
// absent tree means there is no observation to attribute, which is an error,
// not something to paper over. Under dryRun it returns the path and writes
// nothing.
func WriteAttribution(tenantDir string, a *Attribution, dryRun bool) (string, error) {
	p := AuditPath(tenantDir)
	if dryRun {
		return p, nil
	}
	driftDir := filepath.Join(tenantDir, DriftDirName)
	if info, err := os.Stat(driftDir); err != nil || !info.IsDir() {
		return p, fmt.Errorf("%w: %s (run 'azure-rd resource drift' first)", ErrNoObservation, driftDir)
	}
	data, err := yaml.Marshal(a)
	if err != nil {
		return p, fmt.Errorf("failed to marshal drift attribution: %w", err)
	}
	if err := docs.WriteFileAtomic(p, data); err != nil {
		return p, fmt.Errorf("failed to write drift attribution: %w", err)
	}
	return p, nil
}
