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
