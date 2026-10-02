package consistency

import (
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
}

// detect returns the mechanical findings and the unknown-value pairs, sorted.
func detect(settings []Setting, scopes map[string]*Scope, eqs []Equivalence) ([]Finding, []UnknownValue) {
	d := &detector{
		settings: settings,
		scopes:   scopes,
		eqs:      newEquivalenceTable(eqs),
		overlaps: map[[2]string]Overlap{},
		findings: map[string]Finding{},
		unknowns: map[string]UnknownValue{},
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
		var idx []int
		seen := map[string]bool{}
		for _, m := range e.Members {
			if seen[m] {
				continue
			}
			seen[m] = true
			idx = append(idx, byKey[m]...)
		}
		d.pairs(idx, e)
	}

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
			if a.Resource == b.Resource {
				continue
			}
			if d.contextOf(a, b) != e {
				continue
			}
			if e == nil && a.Key != b.Key {
				continue
			}
			if a.Resource > b.Resource {
				a, b = b, a
			}
			d.evaluate(a, b, e)
		}
	}
}

// contextOf decides which comparison a pair belongs to: two configurations on
// one key compare on that key; a pair an equivalence joins compares under it;
// any other pair on one key compares on that key.
func (d *detector) contextOf(a, b *Setting) *Equivalence {
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
	d.findings[id] = Finding{
		Kind:       kind,
		Key:        key,
		Overlap:    o.Verdict,
		Confidence: confidence,
		Operator:   operator,
		A:          FindingSide{Resource: a.Resource, SourceKey: a.SourceKey, Value: a.Value},
		B:          FindingSide{Resource: b.Resource, SourceKey: b.SourceKey, Value: b.Value},
	}
}

// judge returns the finding kind of a pair ("" for none); ok is false when a
// value is unknown and the pair can only be listed.
func (d *detector) judge(a, b *Setting, e *Equivalence) (string, bool) {
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

	// A configuration against a compliance requirement that only checks it.
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
