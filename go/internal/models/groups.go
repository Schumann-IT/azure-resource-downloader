package models

// GroupAxisNotApplicable is the reserved value for a grouping axis that does
// not apply to a resource type — a tenant-level singleton, for example, has no
// meaningful platform. It is deliberately distinct from a blank/absent value,
// which means "not yet classified": the former is a category fact, the latter a
// real taxonomy gap, and the index must never merge the two. It is a member of
// both closed vocabularies below.
const GroupAxisNotApplicable = "n/a"

// PlatformGroups is the closed, ordered vocabulary for the platform grouping
// axis. It is the single source of truth for that axis: the documentation
// prompt constrains the model to these values, docs generate-index emits them
// in the index header (in this display order) so consumers order navigation
// from the data, and any value outside this set is rejected. Keep it in display
// order; treat it as read-only.
var PlatformGroups = []string{
	"Windows",
	"macOS",
	"iOS/iPadOS",
	"Android",
	"Linux",
	"Cross-platform",
	GroupAxisNotApplicable,
}

// FunctionGroups is the closed, ordered vocabulary for the function grouping
// axis. Like PlatformGroups it is the single source of truth for its axis:
// prompt-constrained, emitted in the index header in this display order, and
// validated against on the way in. Keep it in display order; treat it as
// read-only.
var FunctionGroups = []string{
	"Identity & access",
	"Compliance",
	"Configuration",
	"Security",
	"Apps",
	"Enrollment",
	"Updates",
	"Scripts",
	"Governance",
	GroupAxisNotApplicable,
}
