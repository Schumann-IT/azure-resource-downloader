package docs

import (
	"fmt"
	"regexp"
)

// taxonomyIDPattern constrains an axis id or a value id to a stable, URL-safe
// token (lowercase alphanumerics and single hyphens). The id becomes a facet
// key or value in a consumer's URL, so it must survive a label rename and never
// carry whitespace or case that a URL would have to escape. It also forbids a
// leading underscore, keeping the `_`-prefixed representation tokens a consumer
// uses (e.g. `_uncategorised`) reserved: a config id can never collide with one.
var taxonomyIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// programmeAxisID is the id of the axis the legacy `programmes:` sugar compiles
// into, and the axis whose membership is mirrored into the transitional
// `programmes`/`groups` index fields.
const programmeAxisID = "programme"

// TaxonomyConfig is the `taxonomy:` section of the config file: a curated,
// operator-maintained classification resolved at index time. It records rules,
// never facts, so revising it never requires re-downloading a tenant — it only
// changes how docs generate-index groups what is already exported. It is read
// with viper.UnmarshalKey (mapstructure, case-insensitive) so it survives
// viper's lowercasing of config keys (e.g. odataType -> odatatype).
type TaxonomyConfig struct {
	Version    int                 `mapstructure:"version"`
	Programmes []TaxonomyProgramme `mapstructure:"programmes"`
	Axes       []TaxonomyAxis      `mapstructure:"axes"`
}

// TaxonomyProgramme is one programme: a stable id, a display label, and the
// match rules that decide membership. A resource joins the programme if ANY of
// its rules matches (rules are OR-ed). It is sugar for a value on the axis with
// id "programme"; see TaxonomyAxis.
type TaxonomyProgramme struct {
	ID    string         `mapstructure:"id"`
	Label string         `mapstructure:"label"`
	Match []TaxonomyRule `mapstructure:"match"`
}

// TaxonomyAxis is one filter axis: a stable id, a display label, and the values
// a resource can be classified into on that axis. The legacy `programmes:` key
// is accepted as sugar for the axis with id "programme"; defining both is a
// conflict, not a merge.
type TaxonomyAxis struct {
	ID     string          `mapstructure:"id"`
	Label  string          `mapstructure:"label"`
	Values []TaxonomyValue `mapstructure:"values"`
}

// TaxonomyValue is one value on an axis: a stable id, a display label, and the
// match rules that decide membership, with the same OR-of-rules / AND-within-a-
// rule semantics as a programme.
type TaxonomyValue struct {
	ID    string         `mapstructure:"id"`
	Label string         `mapstructure:"label"`
	Match []TaxonomyRule `mapstructure:"match"`
}

// TaxonomyRule is one membership rule over a resource's exported facts. Every
// non-empty field must match for the rule to match (fields are AND-ed). name,
// odataType and platforms are treated as regular expressions; type and scope
// are exact matches.
type TaxonomyRule struct {
	Name      string `mapstructure:"name"`
	Type      string `mapstructure:"type"`
	ODataType string `mapstructure:"odataType"`
	Platforms string `mapstructure:"platforms"`
	Scope     string `mapstructure:"scope"`
}

// taxonomy is the parsed and compiled taxonomy: the axis registry in display
// order (the "programme" axis first when it comes from the `programmes:` sugar),
// each with its values and every regex pre-compiled, ready to classify
// resources.
type taxonomy struct {
	axes []compiledAxis
}

// compiledAxis is one axis with its values compiled, in display order.
type compiledAxis struct {
	id     string
	label  string
	values []compiledValue
}

// compiledValue is one axis value with its rules compiled.
type compiledValue struct {
	id    string
	label string
	rules []compiledRule
}

// axisClassification is the set of values one resource matched on one axis, in
// value (display) order. An empty values slice means the resource is
// uncategorised on that axis.
type axisClassification struct {
	axisID string
	values []programmeRef
}

// compiledRule is a rule with its regex fields pre-compiled. A nil regex means
// the field was not set and is not considered.
type compiledRule struct {
	name      *regexp.Regexp
	rtype     string
	odataType *regexp.Regexp
	platforms *regexp.Regexp
	scope     string
}

// taxonomyFacts are the exported facts a rule matches against, assembled per
// resource at index time.
type taxonomyFacts struct {
	name      string
	rtype     string
	odataType string
	platforms string
	scope     string
}

// programmeRef is an axis-value identity (stable id + display label), the shape
// carried both per resource and in the header registry. (Named for the original
// single programme axis; it now serves any axis's values.)
type programmeRef struct {
	id    string
	label string
}

// compileTaxonomy validates a parsed taxonomy and compiles its regexes,
// preserving axis and value order (which is the display order). The legacy
// `programmes:` key is folded in as the axis with id "programme" (prepended when
// present); defining both `programmes:` and an explicit "programme" axis is a
// conflict. A malformed taxonomy — an unusable version, a duplicate or
// non-URL-safe id, a missing label, an axis with no values, a value with no
// rules, an uncompilable regex or an unknown scope — is a fatal error, so a typo
// surfaces immediately rather than silently grouping nothing.
func compileTaxonomy(cfg TaxonomyConfig) (*taxonomy, error) {
	if cfg.Version < 1 {
		return nil, fmt.Errorf("taxonomy version must be >= 1, got %d", cfg.Version)
	}

	// Assemble the axis list: the programmes sugar becomes the "programme" axis
	// (prepended), then the explicit axes in config order.
	axes := make([]TaxonomyAxis, 0, len(cfg.Axes)+1)
	if len(cfg.Programmes) > 0 {
		for _, a := range cfg.Axes {
			if a.ID == programmeAxisID {
				return nil, fmt.Errorf("taxonomy defines both programmes and an axis %q; use one", programmeAxisID)
			}
		}
		axes = append(axes, TaxonomyAxis{ID: programmeAxisID, Label: "Programme", Values: programmesToValues(cfg.Programmes)})
	}
	axes = append(axes, cfg.Axes...)

	seenAxis := make(map[string]bool, len(axes))
	compiled := make([]compiledAxis, 0, len(axes))
	for ai, a := range axes {
		if !taxonomyIDPattern.MatchString(a.ID) {
			return nil, fmt.Errorf("axis %d has an invalid id %q (want lowercase url-safe token)", ai, a.ID)
		}
		if seenAxis[a.ID] {
			return nil, fmt.Errorf("duplicate axis id %q", a.ID)
		}
		seenAxis[a.ID] = true
		if a.Label == "" {
			return nil, fmt.Errorf("axis %q has no label", a.ID)
		}
		if len(a.Values) == 0 {
			return nil, fmt.Errorf("axis %q has no values", a.ID)
		}

		seenVal := make(map[string]bool, len(a.Values))
		values := make([]compiledValue, 0, len(a.Values))
		for _, v := range a.Values {
			if !taxonomyIDPattern.MatchString(v.ID) {
				return nil, fmt.Errorf("axis %q value has an invalid id %q (want lowercase url-safe token)", a.ID, v.ID)
			}
			if seenVal[v.ID] {
				return nil, fmt.Errorf("axis %q has a duplicate value id %q", a.ID, v.ID)
			}
			seenVal[v.ID] = true
			if v.Label == "" {
				return nil, fmt.Errorf("axis %q value %q has no label", a.ID, v.ID)
			}
			if len(v.Match) == 0 {
				return nil, fmt.Errorf("axis %q value %q has no match rules", a.ID, v.ID)
			}

			rules := make([]compiledRule, 0, len(v.Match))
			for j, r := range v.Match {
				cr, err := compileRule(r)
				if err != nil {
					return nil, fmt.Errorf("axis %q value %q rule %d: %w", a.ID, v.ID, j, err)
				}
				rules = append(rules, cr)
			}
			values = append(values, compiledValue{id: v.ID, label: v.Label, rules: rules})
		}
		compiled = append(compiled, compiledAxis{id: a.ID, label: a.Label, values: values})
	}

	return &taxonomy{axes: compiled}, nil
}

// programmesToValues converts the legacy `programmes:` list into axis values,
// one value per programme, preserving order.
func programmesToValues(ps []TaxonomyProgramme) []TaxonomyValue {
	values := make([]TaxonomyValue, 0, len(ps))
	for _, p := range ps {
		values = append(values, TaxonomyValue(p))
	}
	return values
}

// compileRule validates and compiles a single rule. A rule with no fields set
// would match every resource, which is never intended, so it is rejected.
func compileRule(r TaxonomyRule) (compiledRule, error) {
	var cr compiledRule
	set := false

	if r.Name != "" {
		re, err := regexp.Compile("(?i)" + r.Name)
		if err != nil {
			return cr, fmt.Errorf("invalid name regex %q: %w", r.Name, err)
		}
		cr.name = re
		set = true
	}
	if r.Type != "" {
		cr.rtype = r.Type
		set = true
	}
	if r.ODataType != "" {
		re, err := regexp.Compile("(?i)" + r.ODataType)
		if err != nil {
			return cr, fmt.Errorf("invalid odataType regex %q: %w", r.ODataType, err)
		}
		cr.odataType = re
		set = true
	}
	if r.Platforms != "" {
		re, err := regexp.Compile("(?i)" + r.Platforms)
		if err != nil {
			return cr, fmt.Errorf("invalid platforms regex %q: %w", r.Platforms, err)
		}
		cr.platforms = re
		set = true
	}
	if r.Scope != "" {
		if r.Scope != "device" && r.Scope != "user" {
			return cr, fmt.Errorf("invalid scope %q (want device or user)", r.Scope)
		}
		cr.scope = r.Scope
		set = true
	}

	if !set {
		return cr, fmt.Errorf("rule has no fields set")
	}
	return cr, nil
}

// classifyAxes returns, for every axis in display order, the values a resource
// matched (in value display order). Every axis is represented, so an axis with
// an empty values slice signals the resource is uncategorised on that axis.
func (t *taxonomy) classifyAxes(f taxonomyFacts) []axisClassification {
	if t == nil {
		return nil
	}
	out := make([]axisClassification, 0, len(t.axes))
	for _, a := range t.axes {
		var matched []programmeRef
		for _, v := range a.values {
			if v.matches(f) {
				matched = append(matched, programmeRef{id: v.id, label: v.label})
			}
		}
		out = append(out, axisClassification{axisID: a.id, values: matched})
	}
	return out
}

// classify returns the values a resource matched on the "programme" axis, in
// display order — the legacy single-axis view, retained for the transitional
// programmes/groups emission and for direct testing of the alias.
func (t *taxonomy) classify(f taxonomyFacts) []programmeRef {
	a := t.programmeAxis()
	if a == nil {
		return nil
	}
	var matched []programmeRef
	for _, v := range a.values {
		if v.matches(f) {
			matched = append(matched, programmeRef{id: v.id, label: v.label})
		}
	}
	return matched
}

// registry returns the full value registry of the "programme" axis in display
// order, independent of any tenant's matches, so the index can list a programme
// that matched nothing here. Nil when there is no programme axis.
func (t *taxonomy) registry() []programmeRef {
	a := t.programmeAxis()
	if a == nil {
		return nil
	}
	refs := make([]programmeRef, 0, len(a.values))
	for _, v := range a.values {
		refs = append(refs, programmeRef{id: v.id, label: v.label})
	}
	return refs
}

// programmeAxis returns the axis with id "programme", or nil when the taxonomy
// defines no such axis.
func (t *taxonomy) programmeAxis() *compiledAxis {
	if t == nil {
		return nil
	}
	for i := range t.axes {
		if t.axes[i].id == programmeAxisID {
			return &t.axes[i]
		}
	}
	return nil
}

// matches reports whether any of the value's rules matches the facts.
func (v compiledValue) matches(f taxonomyFacts) bool {
	for _, r := range v.rules {
		if r.matches(f) {
			return true
		}
	}
	return false
}

// matches reports whether every set field of the rule matches the facts.
func (r compiledRule) matches(f taxonomyFacts) bool {
	if r.name != nil && !r.name.MatchString(f.name) {
		return false
	}
	if r.rtype != "" && r.rtype != f.rtype {
		return false
	}
	if r.odataType != nil && !r.odataType.MatchString(f.odataType) {
		return false
	}
	if r.platforms != nil && !r.platforms.MatchString(f.platforms) {
		return false
	}
	if r.scope != "" && r.scope != f.scope {
		return false
	}
	return true
}
