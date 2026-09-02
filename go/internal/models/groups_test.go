package models

import (
	"strings"
	"testing"
)

func TestDocumentationGroupsMarker(t *testing.T) {
	marker := DocumentationGroupsMarker()

	if !strings.HasPrefix(marker, DocGroupsMarkerPrefix) {
		t.Errorf("marker %q does not start with prefix %q", marker, DocGroupsMarkerPrefix)
	}
	if !strings.HasSuffix(marker, "-->") {
		t.Errorf("marker %q is not closed with -->", marker)
	}

	// The marker splits into exactly two axes, platform then function, each
	// carrying its whole vocabulary in order.
	body := strings.TrimSuffix(strings.TrimPrefix(marker, DocGroupsMarkerPrefix), "-->")
	axes := strings.Split(body, "|")
	if len(axes) != 2 {
		t.Fatalf("marker must have exactly two axes, got %d: %q", len(axes), marker)
	}

	for _, tc := range []struct {
		prefix string
		want   []string
	}{
		{"platform=", PlatformGroups},
		{"function=", FunctionGroups},
	} {
		var found string
		for _, axis := range axes {
			if strings.HasPrefix(strings.TrimSpace(axis), tc.prefix) {
				found = strings.TrimSpace(axis)
			}
		}
		if found == "" {
			t.Fatalf("marker missing %q axis: %q", tc.prefix, marker)
		}
		values := strings.Split(strings.TrimPrefix(found, tc.prefix), ", ")
		if len(values) != len(tc.want) {
			t.Fatalf("%s axis has %d values, want %d: %q", tc.prefix, len(values), len(tc.want), found)
		}
		for i, v := range tc.want {
			if values[i] != v {
				t.Errorf("%s value %d = %q, want %q", tc.prefix, i, values[i], v)
			}
		}
	}
}

func TestGroupVocabulariesIncludeNotApplicable(t *testing.T) {
	for name, vocab := range map[string][]string{
		"PlatformGroups": PlatformGroups,
		"FunctionGroups": FunctionGroups,
	} {
		var has bool
		for _, v := range vocab {
			if v == GroupAxisNotApplicable {
				has = true
			}
			if strings.Contains(v, ",") {
				t.Errorf("%s value %q contains a comma, which breaks the doc-groups marker separator", name, v)
			}
		}
		if !has {
			t.Errorf("%s does not include the reserved %q value", name, GroupAxisNotApplicable)
		}
	}
}
