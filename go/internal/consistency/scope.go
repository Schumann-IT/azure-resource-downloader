package consistency

import (
	"sort"
	"strings"

	"azure-resource-downloader/internal/docs"
)

// Platform families a resource or an equivalence's enforcement is keyed by.
const (
	PlatformWindows = "windows"
	PlatformMacOS   = "macos"
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
	PlatformLinux   = "linux"
	platformUnknown = "unknown"
)

// Target kinds of an assignment target.
const (
	kindDevice  = "device"
	kindUser    = "user"
	kindUnknown = "unknown"
)

// Overlap verdicts for a resource pair.
const (
	OverlapNone     = "none"
	OverlapPossible = "possible"
	OverlapCertain  = "certain"
)

// Filter modes as recorded on an assignment target.
const (
	filterInclude = "include"
	filterExclude = "exclude"
)

// target is one include or exclude of a resource's scope.
type target struct {
	allDevices bool
	allUsers   bool
	groupID    string
	filterID   string
	filterType string
}

// filterInfo is an assignment filter's platform and rule from its YAML.
type filterInfo struct {
	Platform string
	Rule     string
}

// Scope is one resource's assignment scope: include and exclude sets, filters
// with their mode, the target kind of each group, and the resource's platform.
type Scope struct {
	includes   []target
	excludes   map[string]bool
	groupKinds map[string]string
	filters    map[string]filterInfo
	platforms  []string
}

// kindOf returns the target kind an include reaches.
func (s *Scope) kindOf(t target) string {
	switch {
	case t.allDevices:
		return kindDevice
	case t.allUsers:
		return kindUser
	}
	if k, ok := s.groupKinds[t.groupID]; ok {
		return k
	}
	return kindUnknown
}

// targetKinds returns the sorted, distinct kinds of the scope's includes.
func (s *Scope) targetKinds() []string {
	seen := map[string]bool{}
	for _, t := range s.includes {
		seen[s.kindOf(t)] = true
	}
	return sortedSet(seen)
}

// filterRefs returns the sorted "<filter id>:<mode>" of the scope's includes.
func (s *Scope) filterRefs() []string {
	seen := map[string]bool{}
	for _, t := range s.includes {
		if t.filterID != "" {
			seen[t.filterID+":"+t.filterType] = true
		}
	}
	return sortedSet(seen)
}

// buildScope models a resource's scope from its metadata assignment targets.
func buildScope(entry docs.ResourceMeta, groupKinds map[string]string, filters map[string]filterInfo) *Scope {
	s := &Scope{
		excludes:   map[string]bool{},
		groupKinds: groupKinds,
		filters:    filters,
		platforms:  resourcePlatforms(entry),
	}
	for _, at := range docs.ParseAssignmentTargets(entry.AssignmentTargets) {
		t := target{
			allDevices: at.AllDevices,
			allUsers:   at.AllUsers,
			groupID:    at.GroupID,
			filterID:   at.FilterID,
			filterType: strings.ToLower(at.FilterType),
		}
		if at.Exclude {
			if at.GroupID != "" {
				s.excludes[at.GroupID] = true
			}
			continue
		}
		if !t.allDevices && !t.allUsers && t.groupID == "" {
			continue
		}
		s.includes = append(s.includes, t)
	}
	return s
}

// resourcePlatforms returns the resource's platform families from metadata
// platforms, else the @odata.type prefix, else unknown.
func resourcePlatforms(entry docs.ResourceMeta) []string {
	seen := map[string]bool{}
	for _, p := range strings.FieldsFunc(entry.Platforms, func(r rune) bool { return r == ',' || r == ' ' }) {
		seen[platformFamily(p)] = true
	}
	if len(seen) == 0 && entry.ODataType != "" {
		seen[platformFamily(strings.TrimPrefix(entry.ODataType, "#microsoft.graph."))] = true
	}
	if len(seen) == 0 {
		seen[platformUnknown] = true
	}
	return sortedSet(seen)
}

// platformFamily maps a platform name or an @odata.type local name to its
// family by prefix.
func platformFamily(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, f := range []string{PlatformWindows, PlatformMacOS, PlatformIOS, PlatformAndroid, PlatformLinux} {
		if strings.HasPrefix(n, f) {
			return f
		}
	}
	return platformUnknown
}

// knownPlatforms reports whether every platform of a scope is known.
func knownPlatforms(ps []string) bool {
	for _, p := range ps {
		if p == platformUnknown {
			return false
		}
	}
	return len(ps) > 0
}

func sharePlatform(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// Overlap is the verdict for a resource pair, with what it rests on.
type Overlap struct {
	Verdict  string
	KindsA   []string
	KindsB   []string
	FiltersA []string
	FiltersB []string
}

// overlapOf decides whether two resources' scopes can reach the same device or
// user. Target kind alone never decides none: a user assignment reaches the
// devices its users sign in to.
func overlapOf(a, b *Scope) Overlap {
	o := Overlap{
		Verdict:  OverlapPossible,
		KindsA:   a.targetKinds(),
		KindsB:   b.targetKinds(),
		FiltersA: a.filterRefs(),
		FiltersB: b.filterRefs(),
	}
	switch {
	case knownPlatforms(a.platforms) && knownPlatforms(b.platforms) && !sharePlatform(a.platforms, b.platforms),
		len(a.includes) == 0 || len(b.includes) == 0,
		excludedBy(a, b) || excludedBy(b, a),
		filteredApart(a, b) || filteredApart(b, a):
		o.Verdict = OverlapNone
	case certainOverlap(a, b):
		o.Verdict = OverlapCertain
	}
	return o
}

// excludedBy reports whether every include of x is a group that y excludes,
// with y's exclusion honoured. Intune ignores an exclusion that mixes kinds (a
// known device group excluded from a user include or All users, or the
// reverse), so such an exclusion does not separate the pair; an unknown kind
// counts as matching.
func excludedBy(x, y *Scope) bool {
	for _, t := range x.includes {
		if t.groupID == "" || !y.excludes[t.groupID] {
			return false
		}
		excludedKind := y.kindOf(target{groupID: t.groupID})
		for _, inc := range y.includes {
			if mixedKinds(excludedKind, y.kindOf(inc)) {
				return false
			}
		}
	}
	return true
}

func mixedKinds(a, b string) bool {
	return a != kindUnknown && b != kindUnknown && a != b
}

// filteredApart reports whether every include of x carries a filter in include
// mode that every include of y carries in exclude mode.
func filteredApart(x, y *Scope) bool {
	f := x.includes[0].filterID
	if f == "" {
		return false
	}
	for _, t := range x.includes {
		if t.filterID != f || t.filterType != filterInclude {
			return false
		}
	}
	for _, t := range y.includes {
		if t.filterID != f || t.filterType != filterExclude {
			return false
		}
	}
	return true
}

// certainOverlap: the same group, or All devices on both, or All users on
// both, is included without a filter and excluded on neither side, on the same
// known platform. The built-in targets cannot be excluded as such.
func certainOverlap(a, b *Scope) bool {
	if !knownPlatforms(a.platforms) || !knownPlatforms(b.platforms) || !sharePlatform(a.platforms, b.platforms) {
		return false
	}
	for _, x := range a.includes {
		if x.filterID != "" {
			continue
		}
		for _, y := range b.includes {
			if y.filterID != "" {
				continue
			}
			switch {
			case x.allDevices && y.allDevices, x.allUsers && y.allUsers:
				return true
			case x.groupID != "" && x.groupID == y.groupID && !a.excludes[x.groupID] && !b.excludes[x.groupID]:
				return true
			}
		}
	}
	return false
}

// groupKind classifies a dynamic group's membershipRule: device or user when
// the rule names only that kind's properties, unknown otherwise.
func groupKind(membershipRule string) string {
	rule := strings.ToLower(membershipRule)
	device := strings.Contains(rule, "device.")
	user := strings.Contains(rule, "user.")
	switch {
	case device && !user:
		return kindDevice
	case user && !device:
		return kindUser
	}
	return kindUnknown
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
