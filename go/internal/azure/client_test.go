package azure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// stubCredential implements azcore.TokenCredential without any network: it
// returns the configured error, or a dummy token when err is nil.
type stubCredential struct {
	err error
}

func (s stubCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if s.err != nil {
		return azcore.AccessToken{}, s.err
	}
	return azcore.AccessToken{Token: "token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// TestVerifySession guards the fail-fast behaviour for a missing 'az login'
// session: a credential that cannot produce a token must surface one clear
// error with the sign-in hint, because every later step of a download degrades
// deliberately (warn-and-continue, per-type skips) and would otherwise bury
// the cause.
func TestVerifySession(t *testing.T) {
	t.Run("no usable session fails with the az login hint", func(t *testing.T) {
		cause := errors.New("ERROR: Please run 'az login' to setup account")
		err := VerifySession(context.Background(), stubCredential{err: cause})
		if err == nil {
			t.Fatal("VerifySession() = nil, want an error for a credential without a session")
		}
		if !errors.Is(err, cause) {
			t.Errorf("VerifySession() error does not wrap the credential error: %v", err)
		}
		if !strings.Contains(err.Error(), "az login") {
			t.Errorf("VerifySession() error %q does not carry the 'az login' hint", err)
		}
	})

	t.Run("usable session verifies cleanly", func(t *testing.T) {
		if err := VerifySession(context.Background(), stubCredential{}); err != nil {
			t.Errorf("VerifySession() = %v, want nil for a working credential", err)
		}
	})
}

// TestProbeSession guards the never-prompt contract of the session probe. The
// non-interactive device-code credential surfaces an authentication-required
// error from GetToken instead of starting the sign-in flow; the probe must read
// that — and every other token failure — as "no session" with a reason, never
// as an error that could fail the command. A signed-in developer machine never
// exercises the would-prompt path, so without this fake the blocking behaviour
// would ship untested.
func TestProbeSession(t *testing.T) {
	t.Run("would-prompt credential reads as no session, not an error", func(t *testing.T) {
		// The error a device-code credential with automatic authentication
		// disabled returns when a token request would require interaction.
		cause := errors.New("authentication is required to acquire a token; call Authenticate")
		ok, reason := ProbeSession(context.Background(), stubCredential{err: cause})
		if ok {
			t.Fatal("ProbeSession() = true for a credential that would prompt; the probe must treat it as no session")
		}
		if reason == "" {
			t.Error("ProbeSession() returned no reason; the caller needs one for its omission note")
		}
	})

	t.Run("missing az login session reads as no session", func(t *testing.T) {
		cause := errors.New("ERROR: Please run 'az login' to setup account")
		if ok, _ := ProbeSession(context.Background(), stubCredential{err: cause}); ok {
			t.Error("ProbeSession() = true for a credential without a session")
		}
	})

	t.Run("usable session reads as available", func(t *testing.T) {
		ok, reason := ProbeSession(context.Background(), stubCredential{})
		if !ok {
			t.Errorf("ProbeSession() = false, %q for a working credential", reason)
		}
	})
}
