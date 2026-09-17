// Package version resolves the tool version reported by `--version` and
// recorded in export metadata.
//
// It lives outside package cmd so command packages under cmd/ can read it
// without importing their parent (which would be an import cycle): the root
// command sets its Version from Resolve, and the download command stamps
// Tool into resources/metadata.yaml, both from this single source.
package version

import "runtime/debug"

// version is the tool version. It is empty in a plain `go build` and injected
// at release time via -ldflags "-X azure-resource-downloader/internal/version.version=<v>".
// When unset, Resolve falls back to the module version or VCS revision the Go
// toolchain embeds, so the reported version tracks the binary rather than a
// literal that never moves.
var version = ""

// Resolve returns the tool version, preferring an ldflags-injected value, then
// the module version of a `go install …@version` build, then the embedded VCS
// revision (with a -dirty suffix for uncommitted changes), and finally "dev".
// This is why toolVersion in metadata.yaml actually moves when the binary does —
// letting a later docs step attribute a prompt-hash change to an upgrade rather
// than reporting it as content drift.
func Resolve() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if dirty {
			rev += "-dirty"
		}
		return rev
	}
	return "dev"
}

// Tool returns the tool identifier recorded in export metadata, derived from
// Resolve so the reported version and the recorded one cannot diverge.
func Tool() string {
	return "azure-rd " + Resolve()
}
