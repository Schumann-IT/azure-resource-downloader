package models

import (
	"fmt"
	"strings"
)

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

// DocGroupsMarkerPrefix is the leading token of the doc-groups marker line, used
// to locate it in an assembled doc-prompt.md the same way doc-headings is found.
const DocGroupsMarkerPrefix = "<!-- doc-groups:"

// DocumentationGroupsMarker renders the closed platform and function grouping
// vocabularies as a single machine-readable marker line for a type's
// doc-prompt.md, e.g.:
//
//	<!-- doc-groups: platform=Windows, macOS, … , n/a | function=Compliance, … , n/a -->
//
// The documentation pipeline reads it back to constrain each document's
// model-authored platformGroup/functionGroup frontmatter to these values —
// exactly as doc-headings constrains the H2 sections. Rendering it from the
// PlatformGroups/FunctionGroups constants keeps those the single source of truth,
// so the axis vocabularies live in one place and every doc-prompt.md derives
// from it rather than carrying a literal that can drift. Values are joined with
// ", " (no vocabulary value contains a comma) and the two axes with " | ".
func DocumentationGroupsMarker() string {
	return fmt.Sprintf("%s platform=%s | function=%s -->",
		DocGroupsMarkerPrefix,
		strings.Join(PlatformGroups, ", "),
		strings.Join(FunctionGroups, ", "))
}
