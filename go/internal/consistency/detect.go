package consistency

import (
	"encoding/json"
	"sort"
)

// Finding kinds.
const (
	KindConflict      = "conflict"
	KindDuplicate     = "duplicate"
	KindContradiction = "contradiction"
)

// Finding confidences.
const (
	ConfidenceFirm     = "firm"
	ConfidencePossible = "possible"
)

// equivalenceKeyPrefix prefixes the key of a finding that rests on an
// equivalence.
const equivalenceKeyPrefix = "equivalence:"

// detector holds what the mechanical detection reads.
type detector struct {
	settings []Setting
	scopes   map[string]*Scope
	eqs      *equivalenceTable
	overlaps map[[2]string]Overlap

	findings map[string]Finding
	unknowns map[string]UnknownValue
	// ruledOut holds the distinct resource pairs that reached evaluate but
	// whose overlap is none.
	ruledOut map[[2]string]bool
}

// detect returns the mechanical findings and the unknown-value pairs, sorted,
// and the number of distinct resource pairs the scope check ruled out.
func detect(settings []Setting, scopes map[string]*Scope, eqs []Equivalence) ([]Finding, []UnknownValue, int) {
	d := &detector{
		settings: settings,
		scopes:   scopes,
		eqs:      newEquivalenceTable(eqs),
		overlaps: map[[2]string]Overlap{},
		findings: map[string]Finding{},
		unknowns: map[string]UnknownValue{},
		ruledOut: map[[2]string]bool{},
	}

	byKey := map[string][]int{}
	for i := range settings {
		byKey[settings[i].Key] = append(byKey[settings[i].Key], i)
	}

	// Same-key pairs.
	for _, idx := range byKey {
		d.pairs(idx, nil)
	}
	// Equivalence pairs: every pair of member settings the equivalence joins.
	for _, e := range d.eqs.ordered {
		d.pairs(memberIndexes(e, byKey), e)
	}
	findings, unknowns := d.sorted()
	return findings, unknowns, len(d.ruledOut)
}

// memberIndexes lists the settings of every distinct member of e.
func memberIndexes(e *Equivalence, byKey map[string][]int) []int {
	var idx []int
	seen := map[string]bool{}
	for _, m := range e.Members {
		if seen[m] {
			continue
		}
		seen[m] = true
		idx = append(idx, byKey[m]...)
	}
	return idx
}

// sorted returns the findings and unknown-value pairs in their stable order.
func (d *detector) sorted() ([]Finding, []UnknownValue) {
	findings := make([]Finding, 0, len(d.findings))
	for _, f := range d.findings {
		findings = append(findings, f)
	}
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.A.Resource != b.A.Resource {
			return a.A.Resource < b.A.Resource
		}
		return a.B.Resource < b.B.Resource
	})

	unknowns := make([]UnknownValue, 0, len(d.unknowns))
	for _, u := range d.unknowns {
		unknowns = append(unknowns, u)
	}
	sort.Slice(unknowns, func(i, j int) bool {
		a, b := unknowns[i], unknowns[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.A.Resource != b.A.Resource {
			return a.A.Resource < b.A.Resource
		}
		return a.B.Resource < b.B.Resource
	})
	return findings, unknowns
}

// pairs evaluates every pair of settings from different resources among idx
// that belongs to context e (nil: the exact-key context).
func (d *detector) pairs(idx []int, e *Equivalence) {
	sort.Ints(idx)
	for i := 0; i < len(idx); i++ {
		for j := i + 1; j < len(idx); j++ {
			a, b := &d.settings[idx[i]], &d.settings[idx[j]]
			if !d.inContext(a, b, e) {
				continue
			}
			if a.Resource > b.Resource {
				a, b = b, a
			}
			d.evaluate(a, b, e)
		}
	}
}

// inContext reports whether the pair, from different resources, belongs to
// context e. Two settings on one key never pair when either is an Apple
// payload type: its presence alone says nothing about a conflict.
func (d *detector) inContext(a, b *Setting, e *Equivalence) bool {
	if a.Resource == b.Resource || d.contextOf(a, b) != e {
		return false
	}
	if a.Key == b.Key && (a.PayloadType || b.PayloadType) {
		return false
	}
	return e != nil || a.Key == b.Key
}

// contextOf decides which comparison a pair belongs to: two configurations on
// one key compare on that key; a pair an equivalence joins compares under it;
// any other pair on one key compares on that key. A list-member setting never
// crosses an equivalence.
func (d *detector) contextOf(a, b *Setting) *Equivalence {
	if a.ListMember || b.ListMember {
		return nil
	}
	if a.Key == b.Key && a.Class == ClassConfiguration && b.Class == ClassConfiguration {
		return nil
	}
	return d.eqs.joining(a.Key, b.Key)
}

func (d *detector) overlap(a, b string) Overlap {
	k := [2]string{a, b}
	if o, ok := d.overlaps[k]; ok {
		return o
	}
	o := overlapOf(d.scopes[a], d.scopes[b])
	d.overlaps[k] = o
	return o
}

// evaluate records the finding (or the unknown value) of one pair; a is the
// lower resource key.
func (d *detector) evaluate(a, b *Setting, e *Equivalence) {
	o := d.overlap(a.Resource, b.Resource)
	if o.Verdict == OverlapNone {
		d.ruledOut[[2]string{a.Resource, b.Resource}] = true
		return
	}
	key, operator, confidence := a.Key, "", ConfidenceFirm
	if e != nil {
		key, operator = equivalenceKeyPrefix+e.ID, string(e.Relation)
		if e.Status != StatusVerified {
			confidence = ConfidencePossible
		}
	}
	id := key + "\x1f" + a.Resource + "\x1f" + b.Resource

	kind, ok := d.judge(a, b, e)
	if !ok {
		if _, done := d.unknowns[id]; !done {
			d.unknowns[id] = UnknownValue{
				Key:     key,
				Overlap: o.Verdict,
				A:       UnknownSide{Resource: a.Resource, SourceKey: a.SourceKey},
				B:       UnknownSide{Resource: b.Resource, SourceKey: b.SourceKey},
			}
		}
		return
	}
	if kind == "" {
		return
	}
	if _, done := d.findings[id]; done {
		return
	}
	valueA, valueB := a.Value, b.Value
	if a.ListMember && b.ListMember {
		// Only the shared members: the rest of each list is additive.
		valueA = sharedMembersJSON(a, b)
		valueB = valueA
	}
	d.findings[id] = Finding{
		Kind:       kind,
		Key:        key,
		Overlap:    o.Verdict,
		Confidence: confidence,
		Operator:   operator,
		A:          FindingSide{Resource: a.Resource, SourceKey: a.SourceKey, Value: valueA},
		B:          FindingSide{Resource: b.Resource, SourceKey: b.SourceKey, Value: valueB},
	}
}

// sharedMembers returns the known members both list-member settings carry,
// sorted.
func sharedMembers(a, b *Setting) []string {
	inB := map[string]bool{}
	for _, m := range b.Members {
		inB[m] = true
	}
	var shared []string
	for _, m := range a.Members {
		if inB[m] {
			shared = append(shared, m)
		}
	}
	return shared
}

// sharedMembersJSON renders the shared members as one JSON list; each member
// is already canonical JSON.
func sharedMembersJSON(a, b *Setting) string {
	shared := sharedMembers(a, b)
	raw := make([]json.RawMessage, 0, len(shared))
	for _, m := range shared {
		raw = append(raw, json.RawMessage(m))
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(data)
}

// judgeMembers judges an additive collection key: shared known members are a
// duplicate; disjoint lists are no finding unless a member is unknown. A
// pair where only one side is a member list cannot be compared. Never a
// conflict: the device installs every resource's members side by side.
func judgeMembers(a, b *Setting) (string, bool) {
	if !a.ListMember || !b.ListMember {
		return "", false
	}
	if len(sharedMembers(a, b)) > 0 {
		return KindDuplicate, true
	}
	if a.UnknownMembers || b.UnknownMembers {
		return "", false
	}
	return "", true
}

// judge returns the finding kind of a pair ("" for none); ok is false when a
// value is unknown and the pair can only be listed.
func (d *detector) judge(a, b *Setting, e *Equivalence) (string, bool) {
	if a.ListMember || b.ListMember {
		return judgeMembers(a, b)
	}
	enforces := func(s *Setting) bool {
		if s.Class == ClassConfiguration {
			return true
		}
		return e != nil && e.enforcedOn(d.scopes[s.Resource].platforms)
	}

	if enforces(a) && enforces(b) || e == nil {
		switch compareSettings(a, b) {
		case cmpUnknown:
			return "", false
		case cmpEqual:
			return KindDuplicate, true
		}
		if enforces(a) && enforces(b) {
			return KindConflict, true
		}
		return "", true
	}

	return d.judgeRequirement(a, b, e)
}

// judgeRequirement judges a configuration against a compliance requirement
// that only checks it.
func (d *detector) judgeRequirement(a, b *Setting, e *Equivalence) (string, bool) {
	var config, requirement *Setting
	switch {
	case a.Class == ClassConfiguration && b.Class == ClassRequirement:
		config, requirement = a, b
	case b.Class == ClassConfiguration && a.Class == ClassRequirement:
		config, requirement = b, a
	default:
		return "", true
	}
	if e.Relation == RelationSame {
		return "", true
	}
	pass, ok := satisfies(e.Relation, config, requirement)
	if !ok {
		return "", false
	}
	if pass {
		return "", true
	}
	return KindContradiction, true
}
