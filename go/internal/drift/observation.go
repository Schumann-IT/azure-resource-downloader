package drift

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"azure-resource-downloader/internal/docs"

	"gopkg.in/yaml.v3"
)

// ObservationPath returns the path of a tenant's drift observation metadata.
func ObservationPath(tenantDir string) string {
	return filepath.Join(tenantDir, DriftDirName, ObservationFileName)
}

// ClearTree removes a tenant's drift/ tree entirely, reporting whether it
// existed. A re-baselining resource download calls it after updating
// resources/metadata.yaml: the new baseline supersedes the observation (and the
// analysis artifacts at the tree root) by definition, so keeping them would
// preserve an answer to a question nobody can ask any more. The path is
// constructed here — never derived from any input — so this cannot reach into
// resources/ or docs/.
func ClearTree(tenantDir string) (bool, error) {
	driftDir := filepath.Join(tenantDir, DriftDirName)
	if _, err := os.Stat(driftDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat drift tree: %w", err)
	}
	if err := os.RemoveAll(driftDir); err != nil {
		return false, fmt.Errorf("failed to clear drift tree: %w", err)
	}
	return true, nil
}

// WriteObservation persists a drift observation: it clears the tenant's
// drift/ tree (a drift run owns its observation space, so the tree holds
// exactly the latest observation and can never accumulate stale leftovers),
// writes the payload of every added, changed and renamed resource at its
// mirrored path, and writes the observation metadata at the tree root —
// atomically, and last, so a consumer that sees the metadata can trust every
// payload it names.
//
// It returns the observation exactly as written (ObservedAt and ToolVersion
// stamped) alongside its path — also under dryRun — so anything attributed to
// it afterwards (drift/audit.yaml) carries the same observedAt byte for byte.
//
// Under dryRun nothing is written and nothing is cleared: a previous
// observation stays intact on disk (the caller reports it as not refreshed).
// It never touches resources/ or docs/: --prune remains the only delete path
// inside the export; a drift run deletes only within its own drift/ tree.
func WriteObservation(tenantDir string, rep *Report, observedAt time.Time, toolVersion string, dryRun bool) (Observation, string, error) {
	obs := rep.Observation
	obs.ObservedAt = observedAt.UTC().Format(time.RFC3339)
	obs.ToolVersion = toolVersion

	driftDir := filepath.Join(tenantDir, DriftDirName)
	metaPath := filepath.Join(driftDir, ObservationFileName)

	if dryRun {
		return obs, metaPath, nil
	}

	// Clear the observation space through the single clear implementation, so
	// this delete can never reach into resources/ or docs/.
	if _, err := ClearTree(tenantDir); err != nil {
		return obs, metaPath, err
	}
	if err := os.MkdirAll(driftDir, 0755); err != nil {
		return obs, metaPath, fmt.Errorf("failed to create drift tree: %w", err)
	}

	// Payloads first, metadata last: the metadata names the payloads, so it
	// must never describe files that are not on disk yet.
	for _, key := range obs.Payloads {
		payloadPath := filepath.Join(driftDir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(payloadPath), 0755); err != nil {
			return obs, metaPath, fmt.Errorf("failed to create payload directory for %s: %w", key, err)
		}
		if err := os.WriteFile(payloadPath, rep.PayloadData[key], 0644); err != nil {
			return obs, metaPath, fmt.Errorf("failed to write payload %s: %w", key, err)
		}
	}

	data, err := yaml.Marshal(&obs)
	if err != nil {
		return obs, metaPath, fmt.Errorf("failed to marshal observation: %w", err)
	}

	// Atomic write (temp file in the same directory, then rename), matching
	// the export's metadata.yaml: a run killed mid-write must leave either
	// nothing or the complete observation, never a truncated mix.
	if err := docs.WriteFileAtomic(metaPath, data); err != nil {
		return obs, metaPath, fmt.Errorf("failed to write observation: %w", err)
	}
	return obs, metaPath, nil
}
