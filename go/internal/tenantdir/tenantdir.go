// Package tenantdir decides which tenant directory a run acts on, from the
// domain the operator declared and the domain the signed-in session resolves
// to. It exists because that decision was made three different ways — a flat
// fallback in the run preparation, an offline-or-detect rule in the docs
// commands, and a declared-vs-resolved cross-check in drift — and only the
// cross-check was right. The resolution is a pure function of two strings so it
// is testable without a network and identical for every command.
package tenantdir

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"azure-resource-downloader/internal/config"
)

// State records how much is known about the tenant a run acts on. It is
// reported rather than inferred, because "the operator said so" and "the tenant
// confirmed it" are different guarantees and a run must be able to say which it
// has.
type State int

const (
	// StateVerified: a domain was declared and the signed-in tenant agrees.
	StateVerified State = iota
	// StateDeclaredUnverified: a domain was declared but no session could
	// confirm it (offline, or tenant resolution failed).
	StateDeclaredUnverified
	// StateResolved: no domain was declared; the signed-in tenant decided it.
	StateResolved
)

// ErrMismatch is returned when the declared domain and the signed-in tenant
// disagree. Continuing would write or compare one tenant's data under another
// tenant's directory, so this always refuses.
var ErrMismatch = errors.New("the declared tenant domain does not match the signed-in tenant")

// ErrUnresolved is returned when neither a declared domain nor a resolved one
// is available. There is deliberately no fallback to the bare output
// directory: an export written directly under <output>/ does not match the
// <output>/<domain>/ layout every other command looks for, so it would be
// invisible from the moment it was written.
var ErrUnresolved = errors.New("cannot determine which tenant this run acts on")

// Result is the decided tenant directory and how it was decided.
type Result struct {
	// Dir is <baseOutput>/<Domain>.
	Dir string
	// Domain is the tenant's default domain, also the directory name.
	Domain string
	// State says whether Domain was verified, merely declared, or resolved.
	State State
}

// Unverified reports whether the domain was taken on the operator's word alone,
// so a caller can say so rather than implying the tenant confirmed it.
func (r Result) Unverified() bool { return r.State == StateDeclaredUnverified }

// Resolve decides the tenant directory. declared is the explicitly passed
// --domain (empty when not given); resolved is the domain the signed-in session
// reported (empty when there is no session, or resolution failed). When both
// are present they must agree; the resolved spelling wins, since it is the
// tenant's own.
func Resolve(baseOutput, declared, resolved string) (Result, error) {
	switch {
	case declared != "" && resolved != "":
		if !strings.EqualFold(declared, resolved) {
			return Result{}, fmt.Errorf("%w (--domain is %q, the signed-in tenant is %q)", ErrMismatch, declared, resolved)
		}
		return result(baseOutput, resolved, StateVerified)

	case declared != "":
		return result(baseOutput, declared, StateDeclaredUnverified)

	case resolved != "":
		return result(baseOutput, resolved, StateResolved)

	default:
		return Result{}, fmt.Errorf("%w: pass --domain to name the tenant, or sign in so it can be resolved", ErrUnresolved)
	}
}

// result validates the chosen domain as a single path segment before joining
// it, so no input can escape the output directory.
func result(baseOutput, domain string, state State) (Result, error) {
	if err := config.ValidateDomain(domain); err != nil {
		return Result{}, err
	}
	return Result{
		Dir:    filepath.Join(baseOutput, domain),
		Domain: domain,
		State:  state,
	}, nil
}
