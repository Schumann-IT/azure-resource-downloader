package docs

import (
	"bytes"
	"regexp"
)

// markerComment matches the marked-block delimiters the splice engines manage
// (`<!-- name:start -->` / `<!-- name:end -->`), which StripTemplateHeader must
// never remove.
var markerComment = regexp.MustCompile(`^\s*[\w-]+:(start|end)\s*$`)

// StripTemplateHeader removes a template's self-description from a finished
// prompt: the literal " (template)" suffix of the first H1 line, and the first
// HTML comment block — the header explaining how to edit the template, which is
// noise in the output an agent is asked to execute. Marker comments are never
// touched, and a template without a header (e.g. a --prompt override) passes
// through unchanged.
func StripTemplateHeader(out []byte) []byte {
	return stripHeaderComment(stripTemplateH1Suffix(out))
}

// stripTemplateH1Suffix drops the " (template)" suffix from the first line when
// it is an H1 carrying it.
func stripTemplateH1Suffix(out []byte) []byte {
	if !bytes.HasPrefix(out, []byte("# ")) {
		return out
	}
	lineEnd := bytes.IndexByte(out, '\n')
	if lineEnd < 0 {
		lineEnd = len(out)
	}
	const suffix = " (template)"
	line := out[:lineEnd]
	if !bytes.HasSuffix(line, []byte(suffix)) {
		return out
	}
	var b bytes.Buffer
	b.Write(line[:len(line)-len(suffix)])
	b.Write(out[lineEnd:])
	return b.Bytes()
}

// stripHeaderComment removes the first HTML comment block unless it is a marker
// delimiter, normalizing the seam to one blank line.
func stripHeaderComment(out []byte) []byte {
	start := bytes.Index(out, []byte("<!--"))
	if start < 0 {
		return out
	}
	rel := bytes.Index(out[start:], []byte("-->"))
	if rel < 0 {
		return out
	}
	if markerComment.Match(out[start+4 : start+rel]) {
		return out
	}
	prefix := bytes.TrimRight(out[:start], "\n")
	suffix := bytes.TrimLeft(out[start+rel+3:], "\n")
	var b bytes.Buffer
	b.Write(prefix)
	if len(prefix) > 0 && len(suffix) > 0 {
		b.WriteString("\n\n")
	}
	b.Write(suffix)
	return b.Bytes()
}
