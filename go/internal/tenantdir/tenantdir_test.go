package tenantdir

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestResolve covers the whole decision table. The two refusals are the point of
// the package: a mismatch would write or compare one tenant's data under another
// tenant's directory, and an unresolved tenant used to fall back to the bare
// output directory, producing an export at <output>/resources/ that no other
// command can ever find again.
func TestResolve(t *testing.T) {
	const base = "/tmp/export"

	tests := []struct {
		name       string
		declared   string
		resolved   string
		wantDomain string
		wantState  State
		wantErr    error
	}{
		{
			name:       "declared and confirmed by the session",
			declared:   "contoso.example.com",
			resolved:   "contoso.example.com",
			wantDomain: "contoso.example.com",
			wantState:  StateVerified,
		},
		{
			name:       "the tenant's own spelling wins over the declaration",
			declared:   "CONTOSO.example.com",
			resolved:   "contoso.example.com",
			wantDomain: "contoso.example.com",
			wantState:  StateVerified,
		},
		{
			name:       "declared with no session to confirm it",
			declared:   "contoso.example.com",
			wantDomain: "contoso.example.com",
			wantState:  StateDeclaredUnverified,
		},
		{
			name:       "resolved from the session alone",
			resolved:   "contoso.example.com",
			wantDomain: "contoso.example.com",
			wantState:  StateResolved,
		},
		{
			name:     "declaration contradicts the session",
			declared: "contoso.example.com",
			resolved: "fabrikam.example.com",
			wantErr:  ErrMismatch,
		},
		{
			name:    "nothing to go on",
			wantErr: ErrUnresolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(base, tt.declared, tt.resolved)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Resolve() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() = %v, want no error", err)
			}
			if got.Domain != tt.wantDomain {
				t.Errorf("Domain = %q, want %q", got.Domain, tt.wantDomain)
			}
			if want := filepath.Join(base, tt.wantDomain); got.Dir != want {
				t.Errorf("Dir = %q, want %q", got.Dir, want)
			}
			if got.State != tt.wantState {
				t.Errorf("State = %v, want %v", got.State, tt.wantState)
			}
			if gotUnverified := got.Unverified(); gotUnverified != (tt.wantState == StateDeclaredUnverified) {
				t.Errorf("Unverified() = %v for state %v", gotUnverified, got.State)
			}
		})
	}
}

// TestResolveRejectsPathsAsDomains: the domain becomes a path element, so a
// value that is itself a path must never be accepted — from either input.
func TestResolveRejectsPathsAsDomains(t *testing.T) {
	for _, domain := range []string{"../escape", "sub/dir", "..", "."} {
		t.Run(domain, func(t *testing.T) {
			if _, err := Resolve("/tmp/export", domain, ""); err == nil {
				t.Errorf("Resolve() accepted declared domain %q", domain)
			}
			if _, err := Resolve("/tmp/export", "", domain); err == nil {
				t.Errorf("Resolve() accepted resolved domain %q", domain)
			}
		})
	}
}

// ExampleResolve shows the everyday case: a declared domain confirmed by the
// signed-in tenant, yielding the export directory a run acts on.
func ExampleResolve() {
	target, err := Resolve("./output", "contoso.onmicrosoft.com", "contoso.onmicrosoft.com")
	if err != nil {
		panic(err)
	}
	println(target.Dir, target.State == StateVerified)
}
