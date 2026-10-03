package consistency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// catalogIDPattern constrains every catalog id to the taxonomy's stable,
// URL-safe token: lowercase alphanumerics and single hyphens, no leading or
// trailing hyphen. A finding resting on an equivalence carries its id in its
// key, so the id must survive a description or label rename.
var catalogIDPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// learnPrefix is what every catalog reference must start with: the catalog
// judges against Microsoft Learn, not against recall or a blog.
const learnPrefix = "https://learn.microsoft.com/"

// Catalog section names, used in validation errors.
const (
	sectionEquivalences = "equivalences"
	sectionTopics       = "topics"
	sectionRules        = "rules"
)

// CatalogConfig is the `consistency:` section of the base configuration file:
// an operator-maintained catalog of which settings describe the same control
// across policy types, how resources group into topics, and which cross-type
// relations to check — each with its Microsoft Learn source and a review
// status. It records rules, never facts, so revising it never requires
// re-downloading a tenant. It is read with viper.UnmarshalKey (mapstructure,
// case-insensitive), which survives viper's lowercasing of config keys.
type CatalogConfig struct {
	Version      int                 `mapstructure:"version" json:"version"`
	Equivalences []EquivalenceConfig `mapstructure:"equivalences" json:"equivalences"`
	Topics       []TopicConfig       `mapstructure:"topics" json:"topics"`
	Rules        []RuleConfig        `mapstructure:"rules" json:"rules"`
}

// EquivalenceConfig joins canonical setting keys that describe one control: a
// compliance property and/or configuration keys (Settings Catalog
// settingDefinitionIds, normalised OMA-URIs, "@odata.type#property").
type EquivalenceConfig struct {
	ID string `mapstructure:"id" json:"id"`
	// Description says which control the members describe.
	Description string `mapstructure:"description" json:"description"`
	// Members are canonical keys as the index emits them.
	Members []string `mapstructure:"members" json:"members"`
	// Relation is same, >=, <=, = or required.
	Relation string `mapstructure:"relation" json:"relation"`
	// Enforced says per platform family whether the compliance side is
	// applied to the device, not only evaluated.
	Enforced map[string]bool `mapstructure:"enforced" json:"enforced"`
	// Reference is the Microsoft Learn page supporting the equivalence.
	Reference string `mapstructure:"reference" json:"reference"`
	// Status is verified or verify.
	Status string `mapstructure:"status" json:"status"`
}

// TopicConfig groups resources into a topic. A resource joins it if ANY match
// rule matches; within a rule every set field must match.
type TopicConfig struct {
	ID    string      `mapstructure:"id" json:"id"`
	Label string      `mapstructure:"label" json:"label"`
	Match []TopicRule `mapstructure:"match" json:"match"`
}

// TopicRule is one topic membership rule. name, odataType, platforms and key
// are case-insensitive regular expressions; type is an exact match. key
// matches when any canonical setting key of the resource matches.
type TopicRule struct {
	Name      string `mapstructure:"name" json:"name,omitempty"`
	Type      string `mapstructure:"type" json:"type,omitempty"`
	ODataType string `mapstructure:"odataType" json:"odataType,omitempty"`
	Platforms string `mapstructure:"platforms" json:"platforms,omitempty"`
	Key       string `mapstructure:"key" json:"key,omitempty"`
}

// RuleConfig is one cross-type relation to check: which resources or settings
// sit on each side, what counts as a violation, and which setting wins.
type RuleConfig struct {
	ID          string   `mapstructure:"id" json:"id"`
	Description string   `mapstructure:"description" json:"description"`
	Left        Selector `mapstructure:"left" json:"left"`
	Right       Selector `mapstructure:"right" json:"right"`
	// Violation says what counts as a violation of the rule.
	Violation string `mapstructure:"violation" json:"violation"`
	// Resolution says which setting wins when both sides apply.
	Resolution string `mapstructure:"resolution" json:"resolution"`
	Reference  string `mapstructure:"reference" json:"reference"`
	Status     string `mapstructure:"status" json:"status"`
}

// Selector names one side of a rule: a topic or a list of canonical keys,
// optionally narrowed to one setting class and to platforms (a regex over the
// resource's platform families).
type Selector struct {
	Topic     string   `mapstructure:"topic" json:"topic,omitempty"`
	Keys      []string `mapstructure:"keys" json:"keys,omitempty"`
	Class     string   `mapstructure:"class" json:"class,omitempty"`
	Platforms string   `mapstructure:"platforms" json:"platforms,omitempty"`
}

// CatalogCounts are the number of entries per catalog section.
type CatalogCounts struct {
	Equivalences int `yaml:"equivalences"`
	Topics       int `yaml:"topics"`
	Rules        int `yaml:"rules"`
}

// Compiled is a validated catalog, ready for the analysis: its equivalences in
// the detector's form, its topics with every regex compiled, its rules, and
// the hash that identifies it.
type Compiled struct {
	config       CatalogConfig
	equivalences []Equivalence
	topics       []compiledTopic
	sha256       string
}

// compiledTopic is a topic with its rules compiled.
type compiledTopic struct {
	id    string
	rules []compiledTopicRule
}

// compiledTopicRule is a topic rule with its regexes compiled; a nil regex or
// an empty type means the field is not considered.
type compiledTopicRule struct {
	name      *regexp.Regexp
	rtype     string
	odataType *regexp.Regexp
	platforms *regexp.Regexp
	key       *regexp.Regexp
}

// TopicFacts are what a topic rule matches against for one resource.
type TopicFacts struct {
	// Name is the resource's display name.
	Name string
	// Type is the resource type, e.g. Microsoft.Graph/deviceConfigurations.
	Type string
	// ODataType is the resource's @odata.type.
	ODataType string
	// Platforms is the resource's platforms fact.
	Platforms string
	// Keys are the canonical keys of the resource's indexed settings.
	Keys []string
}

// CompileCatalog validates a parsed catalog and compiles it. Every error names
// the section and the entry id, so a typo surfaces before any sign-in rather
// than silently joining nothing.
func CompileCatalog(cfg CatalogConfig) (*Compiled, error) {
	if cfg.Version < 1 {
		return nil, fmt.Errorf("version must be >= 1, got %d", cfg.Version)
	}
	c := &Compiled{config: cfg}

	if err := checkIDs(sectionEquivalences, len(cfg.Equivalences), func(i int) string { return cfg.Equivalences[i].ID }); err != nil {
		return nil, err
	}
	for _, e := range cfg.Equivalences {
		eq, err := compileEquivalence(e)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", sectionEquivalences, e.ID, err)
		}
		c.equivalences = append(c.equivalences, eq)
	}

	if err := checkIDs(sectionTopics, len(cfg.Topics), func(i int) string { return cfg.Topics[i].ID }); err != nil {
		return nil, err
	}
	topicIDs := map[string]bool{}
	for _, t := range cfg.Topics {
		ct, err := compileTopic(t)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", sectionTopics, t.ID, err)
		}
		c.topics = append(c.topics, ct)
		topicIDs[t.ID] = true
	}

	if err := checkIDs(sectionRules, len(cfg.Rules), func(i int) string { return cfg.Rules[i].ID }); err != nil {
		return nil, err
	}
	for _, r := range cfg.Rules {
		if err := checkRule(r, topicIDs); err != nil {
			return nil, fmt.Errorf("%s %q: %w", sectionRules, r.ID, err)
		}
	}

	sum, err := catalogHash(cfg)
	if err != nil {
		return nil, err
	}
	c.sha256 = sum
	return c, nil
}

// checkIDs validates the ids of one section: the URL-safe pattern, unique.
func checkIDs(section string, n int, id func(int) string) error {
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		v := id(i)
		if !catalogIDPattern.MatchString(v) {
			return fmt.Errorf("%s entry %d has an invalid id %q (want a lowercase url-safe token)", section, i, v)
		}
		if seen[v] {
			return fmt.Errorf("%s has a duplicate id %q", section, v)
		}
		seen[v] = true
	}
	return nil
}

// relations is the closed set of equivalence relations.
var relations = map[Relation]bool{
	RelationSame: true, RelationAtLeast: true, RelationAtMost: true, RelationEqual: true, RelationRequired: true,
}

// platformFamilies is the closed set of platform families an equivalence's
// enforced map may name: the families the scope model derives.
var platformFamilies = map[string]bool{
	PlatformWindows: true, PlatformMacOS: true, PlatformIOS: true, PlatformAndroid: true, PlatformLinux: true,
}

// compileEquivalence validates one equivalence and converts it to the
// detector's form.
func compileEquivalence(e EquivalenceConfig) (Equivalence, error) {
	distinct := map[string]bool{}
	for _, m := range e.Members {
		if err := checkKey(m); err != nil {
			return Equivalence{}, fmt.Errorf("member %w", err)
		}
		distinct[m] = true
	}
	if len(distinct) < 2 {
		return Equivalence{}, fmt.Errorf("needs at least two distinct members, got %d", len(distinct))
	}
	if !relations[Relation(e.Relation)] {
		return Equivalence{}, fmt.Errorf("unknown relation %q (want same, >=, <=, = or required)", e.Relation)
	}
	enforced := make(map[string]bool, len(e.Enforced))
	for p, on := range e.Enforced {
		family := strings.ToLower(p)
		if !platformFamilies[family] {
			return Equivalence{}, fmt.Errorf("enforced names an unknown platform %q (want windows, macos, ios, android or linux)", p)
		}
		enforced[family] = on
	}
	if err := checkSourced(e.Reference, e.Status); err != nil {
		return Equivalence{}, err
	}
	return Equivalence{
		ID:       e.ID,
		Members:  sortedSet(distinct),
		Relation: Relation(e.Relation),
		Enforced: enforced,
		Status:   e.Status,
	}, nil
}

// checkKey validates one member or selector key against the index's
// canonical forms, naming the form to paste when one can be derived.
func checkKey(key string) error {
	ok, canonical := validMemberKey(key)
	switch {
	case ok:
		return nil
	case canonical != "":
		return fmt.Errorf("%q is not a canonical key; use %q", key, canonical)
	default:
		return fmt.Errorf("%q is not a canonical key (want a lowercase Settings Catalog id, a normalised OMA-URI, \"#microsoft.graph.<Type>#<property>\" or an intent id)", key)
	}
}

// checkSourced validates the Microsoft Learn reference and the review status.
func checkSourced(reference, status string) error {
	if !strings.HasPrefix(reference, learnPrefix) {
		return fmt.Errorf("reference %q must start with %s", reference, learnPrefix)
	}
	if status != StatusVerified && status != StatusVerify {
		return fmt.Errorf("unknown status %q (want verified or verify)", status)
	}
	return nil
}

// compileTopic validates one topic and compiles its match rules.
func compileTopic(t TopicConfig) (compiledTopic, error) {
	if strings.TrimSpace(t.Label) == "" {
		return compiledTopic{}, fmt.Errorf("has no label")
	}
	if len(t.Match) == 0 {
		return compiledTopic{}, fmt.Errorf("has no match rules")
	}
	ct := compiledTopic{id: t.ID}
	for i, r := range t.Match {
		cr, err := compileTopicRule(r)
		if err != nil {
			return compiledTopic{}, fmt.Errorf("match rule %d: %w", i, err)
		}
		ct.rules = append(ct.rules, cr)
	}
	return ct, nil
}

// compileTopicRule compiles one rule; a rule with no field set would match
// every resource, which is never intended, so it is rejected.
func compileTopicRule(r TopicRule) (compiledTopicRule, error) {
	cr := compiledTopicRule{rtype: r.Type}
	for _, f := range []struct {
		field, pattern string
		dst            **regexp.Regexp
	}{
		{"name", r.Name, &cr.name},
		{"odataType", r.ODataType, &cr.odataType},
		{"platforms", r.Platforms, &cr.platforms},
		{"key", r.Key, &cr.key},
	} {
		re, err := compileOptional(f.field, f.pattern)
		if err != nil {
			return cr, err
		}
		*f.dst = re
	}
	if cr.name == nil && cr.rtype == "" && cr.odataType == nil && cr.platforms == nil && cr.key == nil {
		return cr, fmt.Errorf("sets no field (want name, type, odataType, platforms or key)")
	}
	return cr, nil
}

// compileOptional compiles a case-insensitive regex; an empty pattern is nil.
func compileOptional(field, pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid %s regex %q: %w", field, pattern, err)
	}
	return re, nil
}

// checkRule validates one rule against the topics that exist.
func checkRule(r RuleConfig, topics map[string]bool) error {
	if strings.TrimSpace(r.Description) == "" {
		return fmt.Errorf("has no description")
	}
	for _, side := range []struct {
		name string
		sel  Selector
	}{{"left", r.Left}, {"right", r.Right}} {
		if err := checkSelector(side.sel, topics); err != nil {
			return fmt.Errorf("%s: %w", side.name, err)
		}
	}
	if strings.TrimSpace(r.Violation) == "" {
		return fmt.Errorf("has no violation")
	}
	if strings.TrimSpace(r.Resolution) == "" {
		return fmt.Errorf("has no resolution")
	}
	return checkSourced(r.Reference, r.Status)
}

// checkSelector validates one side of a rule: exactly one of an existing topic
// or a non-empty list of canonical keys, a known class, a compilable platforms
// regex.
func checkSelector(s Selector, topics map[string]bool) error {
	switch {
	case s.Topic != "" && len(s.Keys) > 0:
		return fmt.Errorf("names both a topic and keys; use one")
	case s.Topic != "":
		if !topics[s.Topic] {
			return fmt.Errorf("names topic %q, which the catalog does not define", s.Topic)
		}
	case len(s.Keys) > 0:
		for _, k := range s.Keys {
			if err := checkKey(k); err != nil {
				return fmt.Errorf("key %w", err)
			}
		}
	default:
		return fmt.Errorf("names neither a topic nor keys")
	}
	if s.Class != "" && s.Class != string(ClassRequirement) && s.Class != string(ClassConfiguration) {
		return fmt.Errorf("unknown class %q (want requirement or configuration)", s.Class)
	}
	if _, err := compileOptional("platforms", s.Platforms); err != nil {
		return err
	}
	return nil
}

// Equivalences returns the compiled equivalences in the detector's form.
func (c *Compiled) Equivalences() []Equivalence {
	return append([]Equivalence(nil), c.equivalences...)
}

// SHA256 identifies the catalog: the hash of its canonical JSON, so neither
// YAML key order nor entry order moves it, while any change to an entry —
// a description, a reference or a status included — does.
func (c *Compiled) SHA256() string {
	return c.sha256
}

// Counts returns the number of entries per section.
func (c *Compiled) Counts() CatalogCounts {
	return CatalogCounts{
		Equivalences: len(c.config.Equivalences),
		Topics:       len(c.config.Topics),
		Rules:        len(c.config.Rules),
	}
}

// VerifyCount returns the number of equivalences and rules still marked
// status: verify, whose findings are never firm.
func (c *Compiled) VerifyCount() int {
	n := 0
	for _, e := range c.config.Equivalences {
		if e.Status == StatusVerify {
			n++
		}
	}
	for _, r := range c.config.Rules {
		if r.Status == StatusVerify {
			n++
		}
	}
	return n
}

// members returns every distinct equivalence member, sorted.
func (c *Compiled) members() []string {
	seen := map[string]bool{}
	for _, e := range c.equivalences {
		for _, m := range e.Members {
			seen[m] = true
		}
	}
	return sortedSet(seen)
}

// Topics returns the ids of every topic the resource matches, sorted by id. A
// resource may sit in several topics, or in none.
func (c *Compiled) Topics(r TopicFacts) []string {
	var out []string
	for _, t := range c.topics {
		for _, rule := range t.rules {
			if rule.matches(r) {
				out = append(out, t.id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// matches reports whether every set field of the rule matches the resource.
func (r compiledTopicRule) matches(f TopicFacts) bool {
	if r.name != nil && !r.name.MatchString(f.Name) {
		return false
	}
	if r.rtype != "" && r.rtype != f.Type {
		return false
	}
	if r.odataType != nil && !r.odataType.MatchString(f.ODataType) {
		return false
	}
	if r.platforms != nil && !r.platforms.MatchString(f.Platforms) {
		return false
	}
	if r.key != nil && !anyMatches(r.key, f.Keys) {
		return false
	}
	return true
}

func anyMatches(re *regexp.Regexp, values []string) bool {
	for _, v := range values {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}

// catalogHash hashes the catalog's canonical JSON: entries sorted by id per
// section, member and key lists sorted and de-duplicated, topic match rules
// sorted (they are OR-ed, so their order means nothing), map keys sorted by
// encoding/json.
func catalogHash(cfg CatalogConfig) (string, error) {
	canon := CatalogConfig{Version: cfg.Version}
	for _, e := range cfg.Equivalences {
		e.Members = sortedUnique(e.Members)
		enforced := make(map[string]bool, len(e.Enforced))
		for p, on := range e.Enforced {
			enforced[strings.ToLower(p)] = on
		}
		e.Enforced = enforced
		canon.Equivalences = append(canon.Equivalences, e)
	}
	for _, t := range cfg.Topics {
		t.Match = append([]TopicRule(nil), t.Match...)
		sort.SliceStable(t.Match, func(i, j int) bool {
			a, _ := json.Marshal(t.Match[i])
			b, _ := json.Marshal(t.Match[j])
			return string(a) < string(b)
		})
		canon.Topics = append(canon.Topics, t)
	}
	for _, r := range cfg.Rules {
		r.Left.Keys = sortedUnique(r.Left.Keys)
		r.Right.Keys = sortedUnique(r.Right.Keys)
		canon.Rules = append(canon.Rules, r)
	}
	sort.Slice(canon.Equivalences, func(i, j int) bool { return canon.Equivalences[i].ID < canon.Equivalences[j].ID })
	sort.Slice(canon.Topics, func(i, j int) bool { return canon.Topics[i].ID < canon.Topics[j].ID })
	sort.Slice(canon.Rules, func(i, j int) bool { return canon.Rules[i].ID < canon.Rules[j].ID })

	data, err := json.Marshal(canon)
	if err != nil {
		return "", fmt.Errorf("failed to hash the consistency catalog: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// sortedUnique returns the distinct values sorted; nil for none.
func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		seen[v] = true
	}
	return sortedSet(seen)
}
