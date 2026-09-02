package docs

import (
	"fmt"
	"regexp"
)

// programmeIDPattern constrains a programme id to a stable, URL-safe token
// (lowercase alphanumerics and single hyphens). The id becomes a facet value in
// a consumer's URL, so it must survive a label rename and never carry
// whitespace or case that a URL would have to escape.
var programmeIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// TaxonomyConfig is the `taxonomy:` section of the config file: a curated,
// operator-maintained classification resolved at index time. It records rules,
// never facts, so revising it never requires re-downloading a tenant — it only
// changes how docs generate-index groups what is already exported. It is read
// with viper.UnmarshalKey (mapstructure, case-insensitive) so it survives
// viper's lowercasing of config keys (e.g. odataType -> odatatype).
type TaxonomyConfig struct {
	Version    int                 `mapstructure:"version"`
	Programmes []TaxonomyProgramme `mapstructure:"programmes"`
}

// TaxonomyProgramme is one programme: a stable id, a display label, and the
// match rules that decide membership. A resource joins the programme if ANY of
// its rules matches (rules are OR-ed).
type TaxonomyProgramme struct {
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

// taxonomy is the parsed and compiled taxonomy: the programme registry in
// display order with every regex pre-compiled, ready to classify resources.
type taxonomy struct {
	programmes []compiledProgramme
}

// compiledProgramme is a programme with its rules compiled.
type compiledProgramme struct {
	id    string
	label string
	rules []compiledRule
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

// programmeRef is a programme identity (stable id + display label), the shape
// carried both per resource and in the header registry.
type programmeRef struct {
	id    string
	label string
}

// compileTaxonomy validates a parsed taxonomy and compiles its regexes,
// preserving programme order (which is the display order). A malformed
// taxonomy — an unusable version, a duplicate or non-URL-safe id, an empty
// rule, an uncompilable regex or an unknown scope — is a fatal error, so a typo
// surfaces immediately rather than silently grouping nothing.
func compileTaxonomy(cfg TaxonomyConfig) (*taxonomy, error) {
	if cfg.Version < 1 {
		return nil, fmt.Errorf("taxonomy version must be >= 1, got %d", cfg.Version)
	}

	seen := make(map[string]bool, len(cfg.Programmes))
	programmes := make([]compiledProgramme, 0, len(cfg.Programmes))
	for i, p := range cfg.Programmes {
		if !programmeIDPattern.MatchString(p.ID) {
			return nil, fmt.Errorf("programme %d has an invalid id %q (want lowercase url-safe token)", i, p.ID)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("duplicate programme id %q", p.ID)
		}
		seen[p.ID] = true
		if p.Label == "" {
			return nil, fmt.Errorf("programme %q has no label", p.ID)
		}
		if len(p.Match) == 0 {
			return nil, fmt.Errorf("programme %q has no match rules", p.ID)
		}

		rules := make([]compiledRule, 0, len(p.Match))
		for j, r := range p.Match {
			cr, err := compileRule(r)
			if err != nil {
				return nil, fmt.Errorf("programme %q rule %d: %w", p.ID, j, err)
			}
			rules = append(rules, cr)
		}
		programmes = append(programmes, compiledProgramme{id: p.ID, label: p.Label, rules: rules})
	}

	return &taxonomy{programmes: programmes}, nil
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

// classify returns the programmes a resource belongs to, in registry (display)
// order, so a resource's groups list is deterministic. A resource matching no
// programme yields an empty slice, which the caller treats as uncategorised.
func (t *taxonomy) classify(f taxonomyFacts) []programmeRef {
	if t == nil {
		return nil
	}
	var matched []programmeRef
	for _, p := range t.programmes {
		if p.matches(f) {
			matched = append(matched, programmeRef{id: p.id, label: p.label})
		}
	}
	return matched
}

// registry returns the full programme registry in display order, independent of
// any tenant's matches, so the index can list a programme that matched nothing
// here.
func (t *taxonomy) registry() []programmeRef {
	if t == nil {
		return nil
	}
	refs := make([]programmeRef, 0, len(t.programmes))
	for _, p := range t.programmes {
		refs = append(refs, programmeRef{id: p.id, label: p.label})
	}
	return refs
}

// matches reports whether any of the programme's rules matches the facts.
func (p compiledProgramme) matches(f taxonomyFacts) bool {
	for _, r := range p.rules {
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
