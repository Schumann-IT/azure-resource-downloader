package consistency

import (
	"sort"
	"strconv"
	"strings"
)

// Relation is how the members of an equivalence relate: same (both sides
// enforce the control) or a check operator with the compliance side as the
// requirement.
type Relation string

// The closed set of relations.
const (
	RelationSame     Relation = "same"
	RelationAtLeast  Relation = ">="
	RelationAtMost   Relation = "<="
	RelationEqual    Relation = "="
	RelationRequired Relation = "required"
)

// Equivalence statuses: verified against a source, or still to verify (a
// finding resting on it is never firm).
const (
	StatusVerified = "verified"
	StatusVerify   = "verify"
)

// Equivalence joins canonical keys that describe the same control — a
// compliance property and/or configuration keys, each in the index's canonical
// form. It is the alias hook the configuration catalog's equivalences feed.
type Equivalence struct {
	// ID names the equivalence; findings resting on it use key
	// "equivalence:<ID>".
	ID string
	// Members are canonical keys (Settings Catalog ids, normalised OMA-URIs,
	// "@odata.type#property").
	Members []string
	// Relation is same or the requirement's check operator.
	Relation Relation
	// Enforced says per platform family whether the compliance side is applied
	// to the device, not only evaluated.
	Enforced map[string]bool
	// Status is verified or verify.
	Status string
}

// equivalenceTable indexes equivalences by member key.
type equivalenceTable struct {
	byMember map[string][]*Equivalence
	ordered  []*Equivalence
}

func newEquivalenceTable(eqs []Equivalence) *equivalenceTable {
	t := &equivalenceTable{byMember: map[string][]*Equivalence{}}
	for i := range eqs {
		t.ordered = append(t.ordered, &eqs[i])
	}
	sort.SliceStable(t.ordered, func(i, j int) bool { return t.ordered[i].ID < t.ordered[j].ID })
	for _, e := range t.ordered {
		seen := map[string]bool{}
		for _, m := range e.Members {
			if seen[m] {
				continue
			}
			seen[m] = true
			t.byMember[m] = append(t.byMember[m], e)
		}
	}
	return t
}

// joining returns the first equivalence (by id) containing both keys.
func (t *equivalenceTable) joining(a, b string) *Equivalence {
	for _, e := range t.byMember[a] {
		for _, m := range e.Members {
			if m == b {
				return e
			}
		}
	}
	return nil
}

// enforcedOn reports whether the compliance side of e is applied to the device
// on every platform of the resource; an unknown platform is never enforced.
func (e *Equivalence) enforcedOn(platforms []string) bool {
	if e.Relation == RelationSame {
		return true
	}
	if !knownPlatforms(platforms) {
		return false
	}
	for _, p := range platforms {
		if !e.Enforced[p] {
			return false
		}
	}
	return true
}

// comparison is the outcome of comparing two values.
type comparison int

const (
	cmpUnknown comparison = iota
	cmpEqual
	cmpDiffer
)

// compareSettings compares two settings' values: by canonical value within one
// family and key, by scalar form otherwise (the bridge, an equivalence).
func compareSettings(a, b *Setting) comparison {
	if a.Unknown || b.Unknown {
		return cmpUnknown
	}
	if a.Key == b.Key && a.family == b.family {
		if a.Value == b.Value {
			return cmpEqual
		}
		return cmpDiffer
	}
	if a.Scalar == "" || b.Scalar == "" {
		return cmpUnknown
	}
	if scalarsEqual(a.Scalar, b.Scalar) {
		return cmpEqual
	}
	return cmpDiffer
}

func scalarsEqual(a, b string) bool {
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		return fa == fb
	}
	return strings.EqualFold(a, b)
}

// satisfies checks a configuration value against a compliance requirement under
// a check operator. ok is false when the values cannot be judged.
func satisfies(rel Relation, config, requirement *Setting) (pass, ok bool) {
	if config.Unknown || requirement.Unknown || config.Scalar == "" {
		return false, false
	}
	switch rel {
	case RelationRequired:
		return truthy(config.Scalar), true
	case RelationEqual:
		if requirement.Scalar == "" {
			return false, false
		}
		return scalarsEqual(config.Scalar, requirement.Scalar), true
	case RelationAtLeast, RelationAtMost:
		c, errC := strconv.ParseFloat(config.Scalar, 64)
		r, errR := strconv.ParseFloat(requirement.Scalar, 64)
		if errC != nil || errR != nil {
			return false, false
		}
		if rel == RelationAtLeast {
			return c >= r, true
		}
		return c <= r, true
	}
	return false, false
}

// truthy reads a scalar as "the control is on".
func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "", "false", "0", "disabled", "off", "no", "notconfigured", "none":
		return false
	}
	return true
}
