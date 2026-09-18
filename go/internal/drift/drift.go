// Package drift compares a tenant's current state against the export already
// on disk and records what was added, changed, renamed or removed as a durable
// per-tenant artifact under <tenant>/drift/. It never writes into resources/
// or docs/: a drift record is an observation *about* the export, not a
// resource and not a document. Re-baselining is a normal resource download.
package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"

	"gopkg.in/yaml.v3"
)

// DriftDirName is the tree a drift run owns exclusively: a sibling of
// resources/ and docs/ under the tenant directory, cleared and rebuilt on
// every (non-dry) run so it holds exactly the latest observation.
const DriftDirName = "drift"

// ObservationFileName is the observation metadata at the root of the drift/
// tree, exactly as the export's own metadata.yaml sits at the root of
// resources/. Payloads are always at least two levels deep
// (<APIType>/<endpoint>/<name>.yaml), so they can never collide with it.
const ObservationFileName = "metadata.yaml"

// ErrNotComparable is returned by Preflight when the baseline attests a
// configuration different from this run's: comparing across a different
// transformer configuration or filter configuration makes every resource look
// drifted, so the command must refuse rather than report noise.
var ErrNotComparable = errors.New("baseline is not comparable with this run's configuration")

// Verdicts a finding can carry.
const (
	VerdictAdded   = "added"
	VerdictChanged = "changed"
	VerdictRenamed = "renamed"
	VerdictRemoved = "removed"
)

// Preflight checks that the export baseline can answer a drift question for
// this run's configuration, before anything is fetched. It returns warnings
// for conditions that degrade the answer without invalidating it (an
// unattested baseline predating the recorded hashes) and an error wrapping
// docs.ErrTenantMismatch or ErrNotComparable when the comparison must not run.
func Preflight(meta docs.Metadata, expectDomain, currentTransformSha, currentFiltersSha string) ([]string, error) {
	var warnings []string

	if expectDomain != "" && meta.Tenant != "" && !strings.EqualFold(meta.Tenant, expectDomain) {
		return nil, fmt.Errorf("%w (metadata tenant %q, resolved %q)", docs.ErrTenantMismatch, meta.Tenant, expectDomain)
	}
	if meta.Tenant == "" {
		warnings = append(warnings, "the export does not record its tenant; the tenant cross-check is skipped")
	}

	// The run-level hash attests the types the last run covered. A mismatch
	// means the last download used a different transform config than this run:
	// the comparison would report a config change as mass drift, so refuse.
	// Per-entry attestation (in Compare) handles mixed baselines from partial
	// runs; an unrecorded run-level hash predates the field and only warns.
	switch {
	case meta.Run.TransformConfigSha256 == "":
		warnings = append(warnings, "the baseline does not attest its transform configuration (written by an older version); entries without a per-entry hash are reported as unattested")
	case meta.Run.TransformConfigSha256 != currentTransformSha:
		return nil, fmt.Errorf("%w: the baseline's transform configuration (%.12s...) differs from this run's (%.12s...); re-download with the current configuration or run drift with the baseline's",
			ErrNotComparable, meta.Run.TransformConfigSha256, currentTransformSha)
	}

	// Filters never rewrite bytes, only presence: a different filter config
	// reads as resources appearing and vanishing, so it refuses the same way.
	switch {
	case meta.Run.FiltersSha256 == "":
		warnings = append(warnings, "the baseline does not attest its resource-filter configuration (written by an older version); filter-driven presence differences cannot be told from drift")
	case meta.Run.FiltersSha256 != currentFiltersSha:
		return nil, fmt.Errorf("%w: the baseline's resource-filter configuration differs from this run's", ErrNotComparable)
	}

	return warnings, nil
}

// Base64FileModeWarning returns a warning when the effective transform
// configuration is base64-decode in file mode with remove-source: the decoded
// content then lives only in the sidecar artifact, which no recorded fact
// covers, so artifact content drift is not detected and the run must say so
// instead of under-reporting.
func Base64FileModeWarning(configs []models.TransformerConfig) string {
	tc := models.GetTransformerConfig(configs, models.TransformerBase64Decode)
	if tc == nil {
		return ""
	}
	cfg := models.ParseBase64DecodeConfig(tc.Config)
	if cfg.Mode == models.Base64ModeFile && cfg.RemoveSource {
		return "base64-decode runs in file mode with remove-source: decoded payloads live only in sidecar artifacts, whose content drift is NOT detected"
	}
	return ""
}

// Options bundles everything Compare needs. Results are the drained transform
// results of the fetch+transform pipeline over TotalRequests requests;
// SkippedTypes and EmptyTypes come from the same listing that built them.
type Options struct {
	// Baseline is the export's recorded facts (resources/metadata.yaml).
	Baseline docs.Metadata
	// ResourcesDir is the export's resources/ tree, read (never written) for
	// the field deltas of changed resources. Empty disables delta computation.
	ResourcesDir string
	// CurrentTransformSha256 is the hash of this run's effective transform
	// configuration; entries whose recorded hash is missing or different are
	// reported as unattested, never counted as drift.
	CurrentTransformSha256 string
	// Scope is what this run was asked to check (mirrors a download's scope).
	Scope docs.RunScope
	// TotalRequests is the number of fetch requests the listing produced.
	TotalRequests int
	// Results are the transform results, exactly one per request.
	Results []*models.TransformResult
	// SkippedTypes are types whose listing failed; their drift is unknown.
	SkippedTypes []models.SkippedType
	// EmptyTypes are types that listed to zero resources (covered).
	EmptyTypes []string
}

// Finding is one drifted resource, keyed in the observation by its payload
// path relative to the drift/ tree (for removals: the baseline key). All
// fields are facts — read from the resource or computed from its bytes.
type Finding struct {
	Verdict    string `yaml:"verdict"`
	ResourceID string `yaml:"resourceId,omitempty"`
	// DisplayName is the current display name (for removals: the recorded one).
	DisplayName string `yaml:"displayName,omitempty"`
	// PreviousDisplayName is set for renames only.
	PreviousDisplayName string `yaml:"previousDisplayName,omitempty"`
	// BaselineKey is the resource's path relative to resources/. For a rename
	// it differs from the finding's key (the payload lands at the new key);
	// for an addition it is empty (there is no baseline file).
	BaselineKey string `yaml:"baselineKey,omitempty"`
	// BaselineSha256 is the recorded hash of the baseline file, so a consumer
	// can verify the file it is about to diff is the one this verdict was
	// decided against. PayloadSha256 is the hash of the payload written under
	// drift/, computed from the same marshalled bytes a download would write.
	BaselineSha256 string `yaml:"baselineSha256,omitempty"`
	PayloadSha256  string `yaml:"payloadSha256,omitempty"`
	// DocPath is the resource's derived document path (docs tree), so a
	// finding joins to its document with no extra wiring on the consuming side.
	DocPath string `yaml:"docPath,omitempty"`
	// Deltas are dotted-path old → new field changes (changed/renamed only),
	// read from the on-disk baseline YAML. DeltaNote explains their absence.
	Deltas    []Delta `yaml:"deltas,omitempty"`
	DeltaNote string  `yaml:"deltaNote,omitempty"`
}

// Delta is a single dotted-path field change with truncated values.
type Delta struct {
	Path string `yaml:"path"`
	Old  string `yaml:"old"`
	New  string `yaml:"new"`
}

// NotComparableEntry names a baseline entry that could not be byte-compared,
// with the reason. These are excluded from the verdict counts: an entry whose
// bytes were produced under a different configuration cannot distinguish a
// config change from a tenant change.
type NotComparableEntry struct {
	Key    string `yaml:"key"`
	Reason string `yaml:"reason"`
}

// Counts summarises an observation. Compared = Unchanged+Changed+Renamed.
type Counts struct {
	Compared   int `yaml:"compared"`
	Unchanged  int `yaml:"unchanged"`
	Changed    int `yaml:"changed"`
	Renamed    int `yaml:"renamed"`
	Added      int `yaml:"added"`
	Removed    int `yaml:"removed"`
	Unattested int `yaml:"unattested"`
	Excluded   int `yaml:"excluded"`
	Failed     int `yaml:"failed"`
}

// Observation is the on-disk shape of drift/metadata.yaml. It is
// self-describing about what it measured: the baseline it was compared
// against, the tenant, run completeness and scope, the verdict counts, the
// types whose drift is unknown — so a consumer can tell that a re-download has
// invalidated it instead of silently trusting a record of a superseded
// baseline. Apart from ObservedAt it is deterministic for an unchanged tenant.
type Observation struct {
	ObservedAt  string         `yaml:"observedAt"`
	Tenant      string         `yaml:"tenant"`
	ToolVersion string         `yaml:"toolVersion"`
	Baseline    BaselineRef    `yaml:"baseline"`
	Run         ObservationRun `yaml:"run"`
	Counts      Counts         `yaml:"counts"`
	// UnknownTypes could not be listed; their drift is unknown, excluded from
	// the totals.
	UnknownTypes []string `yaml:"unknownTypes"`
	// RemovalsSuppressed reports that this run could not assert removals (it
	// was incomplete), so absent resources were NOT reported as removed.
	RemovalsSuppressed bool                 `yaml:"removalsSuppressed"`
	NotComparable      []NotComparableEntry `yaml:"notComparable,omitempty"`
	// FindingsSha256 hashes the findings alone, so "the same drift as last
	// time" is a single comparison and re-running over an unchanged tenant
	// produces identical bytes except the timestamp.
	FindingsSha256 string             `yaml:"findingsSha256"`
	Findings       map[string]Finding `yaml:"findings"`
	// Payloads lists every payload path this observation wrote, so a consumer
	// never trusts a file the named observation did not produce.
	Payloads []string `yaml:"payloads"`
}

// BaselineRef names the baseline an observation was decided against.
type BaselineRef struct {
	GeneratedAt           string `yaml:"generatedAt"`
	ToolVersion           string `yaml:"toolVersion"`
	TransformConfigSha256 string `yaml:"transformConfigSha256"`
}

// ObservationRun records what the drift run itself covered.
type ObservationRun struct {
	Complete         bool           `yaml:"complete"`
	IncompleteReason string         `yaml:"incompleteReason"`
	Scope            docs.ScopeMeta `yaml:"scope"`
}

// Report is Compare's result: the observation to persist, the payload bytes
// keyed by their drift-tree path, and console-facing conveniences.
type Report struct {
	Observation Observation
	// PayloadData holds the marshalled bytes of every added, changed and
	// renamed resource, keyed by the same path recorded in the observation.
	PayloadData map[string][]byte
	// DriftFound reports whether any verdict was decided (added, changed,
	// renamed or removed).
	DriftFound bool
	// Warnings are conditions the caller should log (delta failures etc.).
	Warnings []string
}

// Compare decides verdicts from the baseline metadata and the freshly
// transformed results: unchanged / changed / added / removed / renamed,
// matching on resourceId first so a renamed resource is reported as a rename
// rather than an add plus a remove. It reads the on-disk resource YAML only
// for changed resources (the delta detail tier); the verdicts themselves need
// no file reads.
func Compare(opts Options) *Report {
	meta := opts.Baseline
	rep := &Report{PayloadData: map[string][]byte{}}
	obs := &rep.Observation
	obs.Tenant = meta.Tenant
	obs.Baseline = BaselineRef{
		GeneratedAt:           meta.GeneratedAt,
		ToolVersion:           meta.ToolVersion,
		TransformConfigSha256: opts.CurrentTransformSha256,
	}
	obs.Findings = map[string]Finding{}
	obs.Run.Scope = docs.ScopeMeta{
		Types:         sortedCopy(opts.Scope.Types),
		ResourceIds:   sortedCopy(opts.Scope.ResourceIDs),
		ResourceGroup: opts.Scope.ResourceGroup,
	}

	// Index baseline entries by resource id. Entries already known absent are
	// excluded from matching (and from removal): the export itself no longer
	// counts them as present, so a live counterpart is an addition.
	keyByID := map[string]string{}
	for key, entry := range meta.Resources {
		if entry.ResourceId != "" && entry.PresentInTenant {
			keyByID[entry.ResourceId] = key
		}
	}

	// Reserve every name the baseline already holds, so an added or renamed
	// resource's payload path is the one a subsequent download would actually
	// choose — never a collision resolving differently in the two runs.
	planner := pipeline.NewNamePlanner()
	for key := range meta.Resources {
		planner.ReserveExisting(typeOfKey(key), strings.TrimSuffix(path.Base(key), ".yaml"))
	}

	// Partition the results. touched marks baseline keys re-observed this run
	// in any form (compared, skipped, filtered, failed, unattested): a touched
	// entry is never a removal candidate.
	touched := map[string]bool{}
	touch := func(resourceID string) {
		if key, ok := keyByID[resourceID]; ok {
			touched[key] = true
		}
	}
	cancelled := 0
	var writable []*models.TransformResult
	for _, tr := range opts.Results {
		switch {
		case tr.Cancelled:
			cancelled++
		case tr.Skipped, tr.Filtered:
			obs.Counts.Excluded++
			touch(tr.ResourceID)
		case tr.Error != nil:
			obs.Counts.Failed++
			touch(tr.ResourceID)
		default:
			writable = append(writable, tr)
		}
	}

	// Completeness mirrors the download's: every request produced a result,
	// nothing was cancelled, every type listed.
	var incomplete []string
	if len(opts.Results) != opts.TotalRequests {
		incomplete = append(incomplete, fmt.Sprintf("only %d of %d requests produced a result", len(opts.Results), opts.TotalRequests))
	}
	if cancelled > 0 {
		incomplete = append(incomplete, fmt.Sprintf("%d requests were cancelled", cancelled))
	}
	if len(opts.SkippedTypes) > 0 {
		incomplete = append(incomplete, fmt.Sprintf("%d resource types could not be listed", len(opts.SkippedTypes)))
	}
	obs.Run.Complete = len(incomplete) == 0
	obs.Run.IncompleteReason = strings.Join(incomplete, "; ")

	for _, st := range opts.SkippedTypes {
		obs.UnknownTypes = append(obs.UnknownTypes, st.ResourceType)
	}
	obs.UnknownTypes = sortedCopy(obs.UnknownTypes)
	if obs.UnknownTypes == nil {
		obs.UnknownTypes = []string{}
	}

	// Deterministic naming order, matching the writer's.
	pipeline.SortForNaming(writable)

	for _, tr := range writable {
		data, err := pipeline.MarshalResourceYAML(tr.CleanedData)
		if err != nil {
			obs.Counts.Failed++
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("failed to marshal %s: %v", tr.ResourceID, err))
			touch(tr.ResourceID)
			continue
		}
		payloadSha := sha256Hex(data)

		baseKey, matched := keyByID[tr.ResourceID]
		if !matched {
			key := payloadKey(planner, tr)
			obs.Counts.Added++
			obs.Findings[key] = Finding{
				Verdict:       VerdictAdded,
				ResourceID:    tr.ResourceID,
				DisplayName:   tr.DisplayName,
				PayloadSha256: payloadSha,
				DocPath:       docPath(key),
			}
			rep.PayloadData[key] = data
			continue
		}

		touched[baseKey] = true
		entry := meta.Resources[baseKey]

		// Attestation gate: only bytes produced under this run's transform
		// config are comparable. An unattested entry is reported, never
		// counted as drift — a config change and a tenant change are
		// indistinguishable from its hash.
		switch {
		case entry.SourceSha256 == "":
			obs.Counts.Unattested++
			obs.NotComparable = append(obs.NotComparable, NotComparableEntry{Key: baseKey,
				Reason: "baseline entry records no source hash (filtered or skipped when last covered)"})
			continue
		case entry.TransformConfigSha256 == "":
			obs.Counts.Unattested++
			obs.NotComparable = append(obs.NotComparable, NotComparableEntry{Key: baseKey,
				Reason: "baseline entry predates per-entry config attestation; re-download the type to attest it"})
			continue
		case entry.TransformConfigSha256 != opts.CurrentTransformSha256:
			obs.Counts.Unattested++
			obs.NotComparable = append(obs.NotComparable, NotComparableEntry{Key: baseKey,
				Reason: "baseline entry was written under a different transform configuration"})
			continue
		}

		if entry.SourceSha256 == payloadSha {
			obs.Counts.Unchanged++
			continue
		}

		// A rename is a content change whose key moved: the payload lands at
		// the new key and the finding carries both keys, so old and new bytes
		// stay joinable.
		renamed := entry.DisplayName != "" && tr.DisplayName != "" && entry.DisplayName != tr.DisplayName
		key := baseKey
		f := Finding{
			Verdict:        VerdictChanged,
			ResourceID:     tr.ResourceID,
			DisplayName:    tr.DisplayName,
			BaselineKey:    baseKey,
			BaselineSha256: entry.SourceSha256,
			PayloadSha256:  payloadSha,
		}
		if renamed {
			key = payloadKey(planner, tr)
			f.Verdict = VerdictRenamed
			f.PreviousDisplayName = entry.DisplayName
			obs.Counts.Renamed++
		} else {
			obs.Counts.Changed++
		}
		f.DocPath = docPath(key)
		f.Deltas, f.DeltaNote = deltasAgainstBaseline(opts.ResourcesDir, baseKey, tr.CleanedData)
		obs.Findings[key] = f
		rep.PayloadData[key] = data
	}

	// Removals: only assert that a resource is gone when the run is complete
	// and its type was actually covered — the same rule --prune applies.
	// Entries recorded as filtered or permission-skipped are excluded: their
	// presence was not re-establishable when last covered.
	covered := coveredTypes(opts)
	suppressed := false
	for key, entry := range meta.Resources {
		if touched[key] || !entry.PresentInTenant || entry.Filtered || entry.Skipped {
			continue
		}
		if !covered[typeOfKey(key)] {
			continue
		}
		if !obs.Run.Complete {
			suppressed = true
			continue
		}
		obs.Counts.Removed++
		obs.Findings[key] = Finding{
			Verdict:        VerdictRemoved,
			ResourceID:     entry.ResourceId,
			DisplayName:    entry.DisplayName,
			BaselineKey:    key,
			BaselineSha256: entry.SourceSha256,
			DocPath:        docPath(key),
		}
	}
	obs.RemovalsSuppressed = suppressed

	obs.Counts.Compared = obs.Counts.Unchanged + obs.Counts.Changed + obs.Counts.Renamed
	rep.DriftFound = obs.Counts.Added+obs.Counts.Changed+obs.Counts.Renamed+obs.Counts.Removed > 0

	for key := range rep.PayloadData {
		obs.Payloads = append(obs.Payloads, key)
	}
	obs.Payloads = sortedCopy(obs.Payloads)
	if obs.Payloads == nil {
		obs.Payloads = []string{}
	}
	sortNotComparable(obs.NotComparable)

	obs.FindingsSha256 = hashFindings(obs.Findings)
	return rep
}

// coveredTypes mirrors the export metadata's coverage rule: a type is covered
// when this run's listing succeeded for it (produced results or listed to
// empty) and the run was type-scoped or full — id/group-scoped runs did not
// enumerate any type and therefore cover nothing.
func coveredTypes(opts Options) map[string]bool {
	covered := map[string]bool{}
	if len(opts.Scope.ResourceIDs) != 0 || opts.Scope.ResourceGroup != "" {
		return covered
	}
	for _, tr := range opts.Results {
		if tr.ResourceType != "" && !tr.Cancelled {
			covered[tr.ResourceType] = true
		}
	}
	for _, t := range opts.EmptyTypes {
		covered[t] = true
	}
	for _, st := range opts.SkippedTypes {
		delete(covered, st.ResourceType)
	}
	return covered
}

// payloadKey reserves a collision-free path for a resource the baseline does
// not hold at this name (added or renamed), mirroring resources/ exactly.
func payloadKey(planner *pipeline.NamePlanner, tr *models.TransformResult) string {
	name := planner.Reserve(tr.ResourceType, tr.SanitizedName, tr.ResourceID)
	return path.Join(tr.ResourceType, name+".yaml")
}

// docPath derives a resource's document path: the tree root and extension are
// swapped (resources/x/y.yaml → docs/x/y.md).
func docPath(key string) string {
	return path.Join(docs.DocsDirName, strings.TrimSuffix(key, ".yaml")+".md")
}

// deltasAgainstBaseline reads the baseline YAML for a changed resource and
// computes the dotted-path field deltas against the fresh cleaned data. Every
// failure degrades to a note: the verdict stands on the hashes alone.
func deltasAgainstBaseline(resourcesDir, baselineKey string, current map[string]interface{}) ([]Delta, string) {
	if resourcesDir == "" {
		return nil, "delta computation disabled"
	}
	raw, err := os.ReadFile(filepath.Join(resourcesDir, filepath.FromSlash(baselineKey)))
	if err != nil {
		return nil, fmt.Sprintf("baseline file not readable: %v", err)
	}
	var old map[string]interface{}
	if err := yaml.Unmarshal(raw, &old); err != nil {
		return nil, fmt.Sprintf("baseline file not parseable: %v", err)
	}
	return ComputeDeltas(old, current), ""
}

// typeOfKey derives the resource type from a key ("<type>/<name>.yaml").
func typeOfKey(key string) string {
	return path.Dir(key)
}

// hashFindings hashes the marshalled findings alone, so an unchanged tenant
// yields an identical hash across runs.
func hashFindings(findings map[string]Finding) string {
	data, err := yaml.Marshal(findings)
	if err != nil {
		return ""
	}
	return sha256Hex(data)
}

// sha256Hex returns the hex-encoded SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// sortedCopy returns a sorted copy of in, or nil when empty.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// sortNotComparable orders the unattested entries by key for deterministic
// output.
func sortNotComparable(entries []NotComparableEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
}
