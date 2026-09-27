package docs

import (
	"strings"
	"testing"
)

func TestStripTemplateHeader(t *testing.T) {
	in := "# Some prompt (template)\n\n<!--\nThis file is a TEMPLATE.\nEditing conventions.\n-->\n\nBody.\n\n<!-- worklist:start -->\nX\n<!-- worklist:end -->\n"
	out := string(StripTemplateHeader([]byte(in)))

	if strings.Contains(out, "This file is a TEMPLATE") {
		t.Error("header comment must be stripped")
	}
	if strings.Contains(out, "(template)") {
		t.Error("H1 template suffix must be stripped")
	}
	if !strings.HasPrefix(out, "# Some prompt\n") {
		t.Errorf("H1 must survive without its suffix, got:\n%s", out)
	}
	for _, marker := range []string{"<!-- worklist:start -->", "<!-- worklist:end -->"} {
		if !strings.Contains(out, marker) {
			t.Errorf("marker %q must survive stripping", marker)
		}
	}
	if !strings.Contains(out, "Body.") {
		t.Error("body must survive stripping")
	}
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("stripping must not leave more than one blank line, got:\n%s", out)
	}
}

func TestStripTemplateHeaderNoHeaderIsNoOp(t *testing.T) {
	in := "# Custom prompt\n\nBody.\n\n<!-- worklist:start -->\nX\n<!-- worklist:end -->\n"
	if got := string(StripTemplateHeader([]byte(in))); got != in {
		t.Errorf("a template without a header must pass through unchanged, got:\n%s", got)
	}
}

func TestStripTemplateHeaderCommentAtStart(t *testing.T) {
	in := "<!--\nheader only\n-->\n\nBody.\n"
	out := string(StripTemplateHeader([]byte(in)))
	if strings.Contains(out, "header only") {
		t.Error("leading header comment must be stripped")
	}
	if !strings.HasPrefix(out, "Body.") {
		t.Errorf("body must lead the output, got:\n%s", out)
	}
}
