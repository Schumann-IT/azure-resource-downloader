package azure

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// tokenCacheName isolates azure-rd's persistent token cache from other
// applications using the same OS store.
const tokenCacheName = "azure-rd"

var (
	// persistentCacheFactory builds the OS-protected token cache. It is a
	// variable so tests replace it and never touch a real Keychain, DPAPI file
	// or kernel keyring.
	persistentCacheFactory = newPersistentCache

	// The cache is built once per process: building it round-trips test data
	// through the OS store, and an unavailable store must be reported once,
	// not once per credential.
	tokenCacheOnce sync.Once
	tokenCache     azidentity.Cache
	errTokenCache  error
)

// sharedTokenCache returns the process-wide persistent token cache, building it
// on first use. When no secure store works it warns once and returns the
// error; callers then fall back to a credential without a cache — tokens are
// never stored unencrypted.
func sharedTokenCache() (azidentity.Cache, error) {
	tokenCacheOnce.Do(func() {
		tokenCache, errTokenCache = persistentCacheFactory()
		if errTokenCache != nil {
			logger.Default.Warn(fmt.Sprintf("Token cache unavailable: %s; signing in on every run", errTokenCache))
		}
	})
	return tokenCache, errTokenCache
}

// TokenCacheStatus describes the device-code session of the dedicated app
// registration for the --debug report: "active (record from <date>)" when a
// session record exists and the token cache works, "no session yet (the next
// run signs in once)" when the cache works but nobody signed in yet, and
// "unavailable (<reason>)" when no secure token store exists. It reads only
// local state and never signs in.
func TokenCacheStatus(tenantID, clientID string) string {
	if _, err := sharedTokenCache(); err != nil {
		return fmt.Sprintf("unavailable (%s)", err)
	}
	if info, ok := authRecordInfo(tenantID, clientID); ok {
		return fmt.Sprintf("active (record from %s)", info.ModTime().Format("2006-01-02"))
	}
	return "no session yet (the next run signs in once)"
}

// deviceCodeAuthenticator is the part of *azidentity.DeviceCodeCredential the
// cached session uses; tests substitute a fake.
type deviceCodeAuthenticator interface {
	azcore.TokenCredential
	Authenticate(ctx context.Context, opts *policy.TokenRequestOptions) (azidentity.AuthenticationRecord, error)
}

// cachedDeviceCodeCredential is the device-code credential of a dedicated app
// registration backed by the persistent token cache: a run gets its tokens
// silently whenever the cache holds a usable session, and signs in with device
// code only when it does not (no record yet, refresh token expired or
// revoked). The inner credential never prompts on its own
// (DisableAutomaticAuthentication); the sign-in is this type's decision.
type cachedDeviceCodeCredential struct {
	inner deviceCodeAuthenticator
	// interactive allows the device-code sign-in. The non-interactive variant
	// returns the authentication-required error unchanged instead.
	interactive bool
	tenantID    string
	clientID    string
	// mu serialises sign-ins, so concurrent token requests on a fresh session
	// lead to exactly one device-code prompt.
	mu sync.Mutex
	// signInErr is the failure of the sign-in a waiting worker would otherwise
	// repeat; set under mu, so a declined or failed prompt is not shown once per
	// worker.
	signInErr error
}

var _ azcore.TokenCredential = (*cachedDeviceCodeCredential)(nil)

// GetToken returns a token from the cached session, signing in with device
// code first when the session needs user interaction and this credential is
// interactive. Any other error passes through unchanged.
func (c *cachedDeviceCodeCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	tk, err := c.inner.GetToken(ctx, opts)
	if !c.interactive || !needsSignIn(err) {
		return tk, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Another caller may have signed in while this one waited for the lock.
	if tk, err = c.inner.GetToken(ctx, opts); !needsSignIn(err) {
		return tk, err
	}
	if c.signInErr != nil {
		return azcore.AccessToken{}, fmt.Errorf("device-code sign-in failed: %w", c.signInErr)
	}

	record, err := c.inner.Authenticate(ctx, &policy.TokenRequestOptions{
		Scopes:    []string{graphScope},
		Claims:    opts.Claims,
		EnableCAE: opts.EnableCAE,
		TenantID:  opts.TenantID,
	})
	if err != nil {
		c.signInErr = err
		return azcore.AccessToken{}, fmt.Errorf("device-code sign-in failed: %w", err)
	}
	if err := saveAuthRecord(c.tenantID, c.clientID, record); err != nil {
		// The sign-in succeeded and this run has its tokens; only the next run
		// loses the session, so this is not worth failing the run over.
		logger.Default.Warn("Could not store the sign-in session; the next run signs in again", "reason", err.Error())
	}
	return c.inner.GetToken(ctx, opts)
}

// needsSignIn reports whether err means the cached session cannot produce a
// token without user interaction.
func needsSignIn(err error) bool {
	var required *azidentity.AuthenticationRequiredError
	return errors.As(err, &required)
}

// newDeviceCodeCredential builds the device-code credential for a dedicated app
// registration. With a working persistent token cache it returns the cached
// session (silent while the cache holds a usable session); without one it
// returns today's plain device-code credential, which signs in on every run.
// Either way it makes no network call.
func newDeviceCodeCredential(clientID, tenantID string, disableAutomaticAuth bool) (azcore.TokenCredential, error) {
	opts := &azidentity.DeviceCodeCredentialOptions{
		ClientID: clientID,
		TenantID: tenantID,
		UserPrompt: func(_ context.Context, msg azidentity.DeviceCodeMessage) error {
			logger.Default.Info("Device-code sign-in required", "instructions", msg.Message)
			return nil
		},
		DisableAutomaticAuthentication: disableAutomaticAuth,
	}

	cache, cacheErr := sharedTokenCache()
	if cacheErr == nil {
		opts.Cache = cache
		opts.DisableAutomaticAuthentication = true
		if record, ok := loadAuthRecord(tenantID, clientID); ok {
			opts.AuthenticationRecord = record
		}
	}

	cred, err := azidentity.NewDeviceCodeCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start device-code sign-in: %w (hint: ensure the app registration allows public client flows)", err)
	}
	if cacheErr != nil {
		return cred, nil
	}
	return &cachedDeviceCodeCredential{
		inner:       cred,
		interactive: !disableAutomaticAuth,
		tenantID:    tenantID,
		clientID:    clientID,
	}, nil
}
