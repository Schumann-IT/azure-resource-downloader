package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// MarshalResourceYAML marshals a resource's cleaned data into the exact bytes
// the writer puts on disk. It is the single marshal path shared by the writer
// (which writes and hashes these bytes into sourceSha256) and by any later
// comparison against that hash (the drift check): a second marshal path is
// exactly how the two would diverge, so there must never be one.
func MarshalResourceYAML(cleanedData map[string]interface{}) ([]byte, error) {
	return yaml.Marshal(cleanedData)
}

// NamePlanner reserves collision-free file base names per resource type. The
// first resource to claim a sanitized name keeps it; any later resource whose
// name sanitizes to the same string is given a deterministic discriminator
// derived from its resource ID, so colliding resources are written to distinct
// files instead of silently overwriting one another.
//
// It is shared by the writer (assigning the names a download writes) and the
// drift check (predicting the path a download would choose for a resource the
// export does not hold yet, after reserving every name the export already
// uses). It is safe for concurrent use.
type NamePlanner struct {
	mu   sync.Mutex
	used map[string]bool
}

// NewNamePlanner returns an empty planner.
func NewNamePlanner() *NamePlanner {
	return &NamePlanner{used: map[string]bool{}}
}

// ReserveExisting marks a base name as taken within a resource type without
// assigning it to anything, so later Reserve calls can never claim it. The
// drift check uses it to pre-reserve every name the export already holds.
func (p *NamePlanner) ReserveExisting(resourceType, baseName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used[resourceType+"/"+baseName] = true
}

// Reserve returns a file base name (no extension) for a resource that is
// unique within its resource type. It never overwrites: the returned name is
// guaranteed unused so far.
func (p *NamePlanner) Reserve(resourceType, sanitizedName, resourceID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := func(name string) string { return resourceType + "/" + name }

	if !p.used[key(sanitizedName)] {
		p.used[key(sanitizedName)] = true
		return sanitizedName
	}

	// Collision: append a stable discriminator. A numeric counter guards the
	// (astronomically unlikely) case that the discriminated name is also taken,
	// e.g. two resources sharing both a sanitized name and a resource ID.
	base := sanitizedName + "_" + nameDiscriminator(resourceID)
	candidate := base
	for i := 2; p.used[key(candidate)]; i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	p.used[key(candidate)] = true

	logger.Default.Warn("Resource file name collides with another resource of the same type; using a disambiguated name to avoid overwriting",
		"type", resourceType,
		"name", sanitizedName,
		"resolved_name", candidate,
		"resource_id", resourceID)

	return candidate
}

// SortForNaming orders transform results into the deterministic sequence names
// are assigned in: by (type, sanitized name, resource id). When several display
// names sanitize to the same string, the one that keeps the bare name is always
// the lowest resource id — never whichever the concurrent upstream stages
// happened to deliver first. The writer and the drift check share it so both
// resolve a collision to the same name.
func SortForNaming(results []*models.TransformResult) {
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.SanitizedName != b.SanitizedName {
			return a.SanitizedName < b.SanitizedName
		}
		return a.ResourceID < b.ResourceID
	})
}

// nameDiscriminator derives a short, stable, filesystem-safe token from a
// resource ID, used to disambiguate colliding file names. It is a prefix of the
// SHA-256 of the ID, so it is deterministic for a given resource and does not
// depend on the (non-deterministic) order resources are written.
func nameDiscriminator(resourceID string) string {
	sum := sha256.Sum256([]byte(resourceID))
	return hex.EncodeToString(sum[:])[:8]
}
