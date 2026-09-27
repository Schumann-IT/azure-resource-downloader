package drift

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// analyzeDriftTemplate is the default template spliced by GenerateAnalyzePrompt.
// This embedded file is the source of truth for the drift-analysis prompt; edit
// it directly. (It must live inside this package because go:embed cannot reach
// outside the package directory.)
//
//go:embed analyze_drift_template.md
var analyzeDriftTemplate []byte

// DefaultAnalyzeTemplate returns the embedded default template bytes.
func DefaultAnalyzeTemplate() []byte {
	out := make([]byte, len(analyzeDriftTemplate))
	copy(out, analyzeDriftTemplate)
	return out
}

// Names of the two analysis artifacts at the drift-tree root, beside the
// observation metadata. Payloads are always at least two levels deep, so they
// can never collide with either. Both are ephemeral: the next resource drift
// run's clear-and-rebuild — and a re-baselining download's ClearTree — sweep
// them with the observation they belong to, along with the per-finding drift
// documents (see ReportPathForKey) the analysis agent writes.
const (
	// AnalyzeFileName is the analysis prompt docs analyze-drift writes.
	AnalyzeFileName = "analyze.md"
	// IndexFileName is the drift summary the analysis agent writes at the tree
	// root — it and the per-finding drift documents are the only files under
	// drift/ that azure-rd itself never produces.
	IndexFileName = "index.md"
)

// ReportPathForKey derives a finding's drift-document path (relative to the
// drift/ tree) from its key: the mirrored resource path with the extension
// swapped, so the drift document sits beside the payload it judges and joins
// to the baseline file, the payload and the documentation document by the same
// key. It is derived — never stored — exactly like a document's docPath, and
// works for every verdict: a removed resource has no payload, but its key
// still names the mirrored path its drift document lands at.
func ReportPathForKey(key string) string {
	return strings.TrimSuffix(key, ".yaml") + ".md"
}

// requiredAnalyzeMarkers are the marked blocks GenerateAnalyzePrompt fills.
var requiredAnalyzeMarkers = []string{"observation", "worklist", "refmap"}

// Sentinel errors let the command map a failure to a distinct exit code.
var (
	// ErrNoObservation is returned when the export has no drift observation to
	// analyze — resource drift has not run (or its tree was cleared).
	ErrNoObservation = errors.New("no drift observation in export directory")
	// ErrObservationSuperseded is returned when the export baseline was
	// re-downloaded after the observation: the findings compare against a
	// baseline that no longer exists, so analyzing them would answer a question
	// nobody can ask any more.
	ErrObservationSuperseded = errors.New("drift observation predates the current export baseline")
	// ErrPayloadMismatch is returned when a payload named by the observation is
	// missing or does not hash to its recorded payloadSha256: the drift tree
	// was tampered with or partially rebuilt, so no payload can be trusted.
	ErrPayloadMismatch = errors.New("drift payload does not match the observation")
)

// AnalyzeOptions bundles everything GenerateAnalyzePrompt needs. It never
// fetches a resource, never authenticates and never writes outside OutPath.
type AnalyzeOptions struct {
	// TenantDir is the per-tenant export directory (the parent of drift/).
	TenantDir string
	// ExpectDomain, when non-empty, must equal the observation's (and the
	// baseline's) tenant field or GenerateAnalyzePrompt refuses.
	ExpectDomain string
	// Template is the prompt template to splice (default or --prompt override).
	Template []byte
	// OutPath is where the finished prompt is written. Empty defaults to
	// TenantDir/drift/analyze.md.
	OutPath string
	// DryRun withholds the write: the comparison and report still run.
	DryRun bool
}

// AnalyzeResult reports what the preflight and rendering found, for the command
// to print and to decide an exit code from.
type AnalyzeResult struct {
	OutPath string
	Written bool
	// IndexPath is where the template directs the agent's summary index; the
	// per-finding drift documents land beside their payloads (ReportPathForKey).
	// Carried here so the command can name it in its output.
	IndexPath string
	// ObservedAt and BaselineGeneratedAt describe the observation under
	// analysis and the baseline it was decided against.
	ObservedAt          string
	BaselineGeneratedAt string
	Counts              Counts
	// NothingToAnalyze reports a clean observation: no findings, no prompt.
	NothingToAnalyze bool
	// MissingSpecTypes are finding types whose doc-prompt.md is absent from the
	// export (e.g. a download run with --no-prompt), so the analysis loses its
	// type-specific lens for them.
	MissingSpecTypes []string
}

// LoadObservation reads a tenant's drift observation (drift/metadata.yaml). It
// returns ErrNoObservation when there is none, so callers can distinguish "no
// drift run yet" from a read failure.
func LoadObservation(tenantDir string) (Observation, error) {
	metaPath := ObservationPath(tenantDir)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Observation{}, fmt.Errorf("%w: %s", ErrNoObservation, metaPath)
		}
		return Observation{}, fmt.Errorf("failed to read drift observation: %w", err)
	}
	var obs Observation
	if err := yaml.Unmarshal(data, &obs); err != nil {
		return Observation{}, fmt.Errorf("failed to parse drift observation: %w", err)
	}
	return obs, nil
}

// GenerateAnalyzePrompt renders the drift-analysis prompt from the latest
// observation and the export baseline. It is fully offline: it reads
// drift/metadata.yaml, resources/metadata.yaml and the payloads it verifies,
// and writes exactly one file (OutPath) unless DryRun is set.
func GenerateAnalyzePrompt(opts AnalyzeOptions) (*AnalyzeResult, error) {
	log := logger.Default

	obs, meta, err := analyzePreflight(opts)
	if err != nil {
		return nil, err
	}

	res := &AnalyzeResult{
		OutPath:             opts.OutPath,
		IndexPath:           filepath.Join(opts.TenantDir, DriftDirName, IndexFileName),
		ObservedAt:          obs.ObservedAt,
		BaselineGeneratedAt: meta.GeneratedAt,
		Counts:              obs.Counts,
	}
	if res.OutPath == "" {
		res.OutPath = filepath.Join(opts.TenantDir, DriftDirName, AnalyzeFileName)
	}

	if len(obs.Findings) == 0 {
		res.NothingToAnalyze = true
		return res, nil
	}

	specPresent := specPresence(opts.TenantDir, &obs)
	res.MissingSpecTypes = missingSpecTypes(specPresent)

	out := opts.Template
	blocks := []struct {
		name    string
		content string
	}{
		{"observation", renderAnalyzeObservation(opts.TenantDir, &obs)},
		{"worklist", renderAnalyzeWorklist(&obs, specPresent)},
		{"refmap", renderAnalyzeRefmap(docs.NewReferenceIndex(&meta))},
	}
	for _, b := range blocks {
		out, err = docs.SpliceMarker(out, b.name, b.content)
		if err != nil {
			return nil, err
		}
	}

	if opts.DryRun {
		log.Info("Dry-run: not writing analysis prompt", "path", res.OutPath, "findings", len(obs.Findings))
		return res, nil
	}

	if err := writeAnalyzePrompt(res.OutPath, out); err != nil {
		return nil, err
	}
	res.Written = true
	return res, nil
}

// analyzePreflight loads and cross-checks the observation and the baseline. It
// refuses (with a sentinel error) when the observation is missing, describes a
// superseded baseline, belongs to a different tenant, or names payloads that do
// not match their recorded hashes — every refusal is a "cannot answer", never a
// degraded answer.
func analyzePreflight(opts AnalyzeOptions) (Observation, docs.Metadata, error) {
	obs, err := LoadObservation(opts.TenantDir)
	if err != nil {
		return Observation{}, docs.Metadata{}, err
	}
	meta, err := docs.LoadExportMetadata(opts.TenantDir)
	if err != nil {
		return Observation{}, docs.Metadata{}, err
	}

	if opts.ExpectDomain != "" && obs.Tenant != "" && !strings.EqualFold(obs.Tenant, opts.ExpectDomain) {
		return Observation{}, docs.Metadata{}, fmt.Errorf("%w (observation tenant %q, resolved %q)", docs.ErrTenantMismatch, obs.Tenant, opts.ExpectDomain)
	}
	if opts.ExpectDomain != "" && meta.Tenant != "" && !strings.EqualFold(meta.Tenant, opts.ExpectDomain) {
		return Observation{}, docs.Metadata{}, fmt.Errorf("%w (metadata tenant %q, resolved %q)", docs.ErrTenantMismatch, meta.Tenant, opts.ExpectDomain)
	}

	// A re-download moves the baseline's generatedAt (partial runs included),
	// which is exactly what makes the observation's verdicts unanswerable.
	if obs.Baseline.GeneratedAt != meta.GeneratedAt {
		return Observation{}, docs.Metadata{}, fmt.Errorf("%w (observation compared against %q, baseline now %q)",
			ErrObservationSuperseded, obs.Baseline.GeneratedAt, meta.GeneratedAt)
	}

	if err := docs.ValidateMarkers(opts.Template, requiredAnalyzeMarkers); err != nil {
		return Observation{}, docs.Metadata{}, err
	}

	if err := verifyPayloads(opts.TenantDir, &obs); err != nil {
		return Observation{}, docs.Metadata{}, err
	}
	return obs, meta, nil
}

// verifyPayloads checks every payload the observation names against its
// recorded hash, so the prompt never directs an agent at bytes the observation
// did not produce.
func verifyPayloads(tenantDir string, obs *Observation) error {
	driftDir := filepath.Join(tenantDir, DriftDirName)
	for _, key := range obs.Payloads {
		f, ok := obs.Findings[key]
		if !ok {
			return fmt.Errorf("%w: %s is not named by any finding", ErrPayloadMismatch, key)
		}
		raw, err := os.ReadFile(filepath.Join(driftDir, filepath.FromSlash(key)))
		if err != nil {
			return fmt.Errorf("%w: %s is not readable (%v)", ErrPayloadMismatch, key, err)
		}
		if sha256Hex(raw) != f.PayloadSha256 {
			return fmt.Errorf("%w: %s does not hash to its recorded payloadSha256", ErrPayloadMismatch, key)
		}
	}
	return nil
}

// specPresence reports, per finding type, whether the export carries the type's
// doc-prompt.md — the type-specific lens the analysis reads.
func specPresence(tenantDir string, obs *Observation) map[string]bool {
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	present := map[string]bool{}
	for key := range obs.Findings {
		rtype := typeOfKey(key)
		if _, known := present[rtype]; known {
			continue
		}
		info, err := os.Stat(filepath.Join(resourcesDir, filepath.FromSlash(rtype), "doc-prompt.md"))
		present[rtype] = err == nil && !info.IsDir()
	}
	return present
}

// missingSpecTypes returns the sorted types whose spec is absent.
func missingSpecTypes(present map[string]bool) []string {
	var missing []string
	for rtype, ok := range present {
		if !ok {
			missing = append(missing, rtype)
		}
	}
	sort.Strings(missing)
	return missing
}

// writeAnalyzePrompt creates the drift/ directory if needed and writes the
// finished prompt. Writing analyze.md is the one file docs analyze-drift adds
// to the drift tree; it never touches the observation or the payloads.
func writeAnalyzePrompt(outPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("failed to create drift directory: %w", err)
	}
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write analysis prompt: %w", err)
	}
	return nil
}
