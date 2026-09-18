package drift

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ObservationPath returns the path of a tenant's drift observation metadata.
func ObservationPath(tenantDir string) string {
	return filepath.Join(tenantDir, DriftDirName, ObservationFileName)
}

// WriteObservation persists a drift observation: it clears the tenant's
// drift/ tree (a drift run owns its observation space, so the tree holds
// exactly the latest observation and can never accumulate stale leftovers),
// writes the payload of every added, changed and renamed resource at its
// mirrored path, and writes the observation metadata at the tree root —
// atomically, and last, so a consumer that sees the metadata can trust every
// payload it names.
//
// Under dryRun nothing is written and nothing is cleared: a previous
// observation stays intact on disk (the caller reports it as not refreshed).
// It never touches resources/ or docs/: --prune remains the only delete path
// inside the export; a drift run deletes only within its own drift/ tree.
func WriteObservation(tenantDir string, rep *Report, observedAt time.Time, toolVersion string, dryRun bool) (string, error) {
	obs := rep.Observation
	obs.ObservedAt = observedAt.UTC().Format(time.RFC3339)
	obs.ToolVersion = toolVersion

	driftDir := filepath.Join(tenantDir, DriftDirName)
	metaPath := filepath.Join(driftDir, ObservationFileName)

	if dryRun {
		return metaPath, nil
	}

	// Clear the observation space. The path is constructed here, always
	// <tenantDir>/drift — never derived from any input — so this cannot reach
	// into resources/ or docs/.
	if err := os.RemoveAll(driftDir); err != nil {
		return metaPath, fmt.Errorf("failed to clear drift tree: %w", err)
	}
	if err := os.MkdirAll(driftDir, 0755); err != nil {
		return metaPath, fmt.Errorf("failed to create drift tree: %w", err)
	}

	// Payloads first, metadata last: the metadata names the payloads, so it
	// must never describe files that are not on disk yet.
	for _, key := range obs.Payloads {
		payloadPath := filepath.Join(driftDir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(payloadPath), 0755); err != nil {
			return metaPath, fmt.Errorf("failed to create payload directory for %s: %w", key, err)
		}
		if err := os.WriteFile(payloadPath, rep.PayloadData[key], 0644); err != nil {
			return metaPath, fmt.Errorf("failed to write payload %s: %w", key, err)
		}
	}

	data, err := yaml.Marshal(&obs)
	if err != nil {
		return metaPath, fmt.Errorf("failed to marshal observation: %w", err)
	}

	// Atomic write (temp file in the same directory, then rename), matching
	// the export's metadata.yaml: a run killed mid-write must leave either
	// nothing or the complete observation, never a truncated mix.
	tmp, err := os.CreateTemp(driftDir, ".metadata-*.yaml")
	if err != nil {
		return metaPath, fmt.Errorf("failed to create temporary observation file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return metaPath, fmt.Errorf("failed to write observation: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return metaPath, fmt.Errorf("failed to write observation: %w", err)
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		_ = os.Remove(tmpPath)
		return metaPath, fmt.Errorf("failed to set observation permissions: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		_ = os.Remove(tmpPath)
		return metaPath, fmt.Errorf("failed to replace observation: %w", err)
	}
	return metaPath, nil
}
