package azure

import (
	"context"
	"sync"

	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// signInLogCredential decorates a credential so the run says "Signed in" when
// the sign-in has actually happened. Credentials are lazy: a device-code
// sign-in takes place at the first token request, well after the credential
// was built, so a log line at construction time would claim a sign-in that has
// not happened yet.
type signInLogCredential struct {
	inner azcore.TokenCredential
	// once is entered only after a successful token request, so a failed
	// first request leaves the line for the first one that succeeds.
	once sync.Once
}

var _ azcore.TokenCredential = (*signInLogCredential)(nil)

// WithSignInLog wraps cred so that its first successful token request logs
// "Signed in" with the account and tenant read from the token's claims —
// exactly once, however many requests run concurrently. It makes no request of
// its own and never logs the token.
func WithSignInLog(cred azcore.TokenCredential) azcore.TokenCredential {
	return &signInLogCredential{inner: cred}
}

// GetToken returns the inner credential's token unchanged, logging the sign-in
// once on the first success.
func (c *signInLogCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	tk, err := c.inner.GetToken(ctx, opts)
	if err != nil {
		return tk, err
	}
	c.once.Do(func() { logSignedIn(tk.Token) })
	return tk, nil
}

// logSignedIn reports the signed-in account from the token's identity claims.
// A token whose claims cannot be read still proves the sign-in, so the line is
// logged without the account rather than not at all.
func logSignedIn(token string) {
	log := logger.Default
	id, err := parseIdentityClaims(token)
	if err != nil {
		log.Debug("Could not read the signed-in account from the token", "error", err)
		log.Info("Signed in")
		return
	}
	log.Info("Signed in", "user", id.Username, "tenant_id", id.TenantID)
}
