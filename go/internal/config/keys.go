// Package config owns where a setting may be written and how the configuration
// files are located and layered. It holds the single table that decides whether
// a key belongs in the shared base file, in a per-tenant profile, or nowhere at
// all because it is a command-line-only switch — so the partition the README
// documents, the example files illustrate and the tests assert all read from
// one place.
package config

import (
	"fmt"
	"sort"
	"strings"
)

// Scope says where a configuration key may legally be set.
type Scope int

const (
	// ScopeGeneral keys describe behaviour, tuning or the export format rather
	// than a tenant, and belong in the shared base file only.
	ScopeGeneral Scope = iota
	// ScopeTenant keys name something that exists inside one tenant, so a
	// value from another tenant is meaningless or harmful. They belong in a
	// per-tenant profile only.
	ScopeTenant
	// ScopeFlagOnly keys are not configuration at all: they are set on the
	// command line. They are listed so a file that still carries one gets a
	// migration message naming the flag instead of a bare "unknown key".
	ScopeFlagOnly
)

// BaseFileName is the file a config directory is expected to hold with the
// general settings, so selecting a tenant needs only --config-dir and --domain.
// It can never collide with a profile: an Entra default domain always contains
// a dot.
const BaseFileName = "base.yaml"

// Built-in defaults for settings that have no flag to carry one. They live here
// rather than as literals at the point of use so a single change cannot leave
// the documentation, the example file and the code disagreeing.
const (
	DefaultWorkerCount    = 5
	DefaultTimeoutSeconds = 300
)

// AuditWorkspaceKey names the Log Analytics workspace id (a GUID) whose
// IntuneAuditLogs and AuditLogs tables attribute drift findings to an actor.
// Unset means attribution is off.
const AuditWorkspaceKey = "audit-workspace-id"

// ExcludeTypeKey names the resource types a tenant never lists — typically the
// ARM types of an Intune/Entra-only tenant whose account holds no subscription
// role. Excluded types are never requested, so leaving them out keeps a run
// complete.
const ExcludeTypeKey = "exclude-type"

// keyScopes is the single truth for the configuration partition. Every key the
// tool recognises appears exactly once; anything absent is rejected as unknown,
// which is what turns a typo into an error instead of a silently ignored line.
var keyScopes = map[string]Scope{
	// Tenant-scoped: profile only.
	"tenant-id":    ScopeTenant,
	"client-id":    ScopeTenant,
	"subscription": ScopeTenant,
	"filters":      ScopeTenant,
	// The Log Analytics workspace holding this tenant's audit tables. A
	// workspace typed for one tenant and forgotten would silently apply to the
	// next and return no rows — which reads as "nobody changed it" — so it is
	// tenant-scoped and has no flag.
	AuditWorkspaceKey: ScopeTenant,
	// The resource types this tenant never lists. Which types a tenant's
	// account can read is a property of that tenant, and the exclusion is
	// recorded in the export metadata where it gates drift comparability, so
	// a shared value would silently re-scope every tenant's baseline.
	ExcludeTypeKey: ScopeTenant,

	// General: base only.
	"output":          ScopeGeneral,
	"type":            ScopeGeneral,
	"workers":         ScopeGeneral,
	"workers-by-api":  ScopeGeneral,
	"timeout":         ScopeGeneral,
	"resolve-secrets": ScopeGeneral,
	"no-prompt":       ScopeGeneral,
	"prune":           ScopeGeneral,
	"transformers":    ScopeGeneral,
	"taxonomy":        ScopeGeneral,

	// Command-line only. Each of these was a config key before the
	// configuration became the single source of truth, so naming them here
	// turns an outdated file into a precise migration message.
	"dry-run":        ScopeFlagOnly,
	"log-level":      ScopeFlagOnly,
	"debug":          ScopeFlagOnly,
	"domain":         ScopeFlagOnly,
	"config":         ScopeFlagOnly,
	"config-dir":     ScopeFlagOnly,
	"resource-id":    ScopeFlagOnly,
	"resource-group": ScopeFlagOnly,
	"out":            ScopeFlagOnly,
	"prompt":         ScopeFlagOnly,
	"exit-code":      ScopeFlagOnly,
}

// ScopeOf reports the scope of a configuration key, and whether it is a key the
// tool recognises at all.
func ScopeOf(key string) (Scope, bool) {
	scope, ok := keyScopes[strings.ToLower(key)]
	return scope, ok
}

// KeysInScope returns the recognised keys of one scope, sorted, so the example
// files, the documentation and the tests can enumerate the partition instead of
// restating it.
func KeysInScope(scope Scope) []string {
	var keys []string
	for key, s := range keyScopes {
		if s == scope {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// ValidateKeys checks the top-level keys of one configuration file against the
// partition. want is the scope the file may carry (ScopeGeneral for a base
// file, ScopeTenant for a profile); every key of the other scope, every
// command-line-only key and every unrecognised key is an error naming the key
// and the file, because a silently ignored setting is indistinguishable from
// one that had no effect.
func ValidateKeys(path string, keys []string, want Scope) error {
	var problems []string
	for _, key := range keys {
		scope, known := ScopeOf(key)
		switch {
		case !known:
			problems = append(problems, fmt.Sprintf("%q is not a known setting", key))
		case scope == ScopeFlagOnly:
			problems = append(problems, fmt.Sprintf("%q is a command-line flag, not a setting: pass --%s", key, key))
		case scope != want:
			problems = append(problems, fmt.Sprintf("%q belongs in %s, not in %s", key, scopeHome(scope), scopeHome(want)))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid configuration in %s: %s", path, strings.Join(problems, "; "))
}

// scopeHome names where a scope's keys live, for error messages.
func scopeHome(scope Scope) string {
	switch scope {
	case ScopeTenant:
		return "a tenant profile (<config-dir>/<domain>.yaml)"
	case ScopeGeneral:
		return "the base configuration file"
	default:
		return "the command line"
	}
}
