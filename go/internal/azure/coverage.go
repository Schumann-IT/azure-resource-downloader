package azure

import "strings"

// PermissionCoverage splits the declared delegated permissions into those the
// token scopes cover and those they miss. Matching is case-insensitive, and a
// ReadWrite scope covers its Read counterpart (DeviceManagementApps.ReadWrite.All
// covers DeviceManagementApps.Read.All). Both results keep the declared order
// and spelling, without duplicates. It does no I/O.
func PermissionCoverage(declared, scopes []string) (covered, missing []string) {
	granted := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			granted[strings.ToLower(scope)] = true
		}
	}

	seen := map[string]bool{}
	for _, permission := range declared {
		key := strings.ToLower(permission)
		if seen[key] {
			continue
		}
		seen[key] = true
		if rw := readWriteCounterpart(key); granted[key] || (rw != "" && granted[rw]) {
			covered = append(covered, permission)
		} else {
			missing = append(missing, permission)
		}
	}
	return covered, missing
}

// readWriteCounterpart returns the ReadWrite form of a lower-cased Read
// permission ("x.read.all" → "x.readwrite.all", "user.read" →
// "user.readwrite"), or "" when the permission has no Read segment.
func readWriteCounterpart(permission string) string {
	segments := strings.Split(permission, ".")
	for i, segment := range segments {
		if segment == "read" {
			segments[i] = "readwrite"
			return strings.Join(segments, ".")
		}
	}
	return ""
}
