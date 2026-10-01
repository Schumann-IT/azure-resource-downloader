package azure

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// useCacheFactory replaces the persistent cache factory for one test and
// resets the process-wide cache, so no test touches a real secure store. It
// returns a counter of factory calls.
func useCacheFactory(t *testing.T, cacheErr error) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	orig := persistentCacheFactory
	persistentCacheFactory = func() (azidentity.Cache, error) {
		calls.Add(1)
		return azidentity.Cache{}, cacheErr
	}
	resetTokenCache()
	t.Cleanup(func() {
		persistentCacheFactory = orig
		resetTokenCache()
	})
	return &calls
}

func resetTokenCache() {
	tokenCacheOnce = sync.Once{}
	tokenCache = azidentity.Cache{}
	errTokenCache = nil
}

// captureLog points the default logger at a buffer for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.Default.SetOutput(&buf)
	t.Cleanup(func() { logger.Default.SetOutput(os.Stderr) })
	return &buf
}

// fakeDeviceCode stands in for *azidentity.DeviceCodeCredential with
// automatic authentication disabled: GetToken fails with an
// authentication-required error until Authenticate ran, then succeeds.
type fakeDeviceCode struct {
	mu        sync.Mutex
	signedIn  bool
	silentErr error
	authErr   error
	authCalls int
	record    azidentity.AuthenticationRecord

	// waitForFailures, when set, makes Authenticate wait until this many
	// GetToken calls have failed, so a concurrency test is sure every caller
	// reached the sign-in path before the first sign-in completes.
	waitForFailures int32
	failures        atomic.Int32
	allFailed       chan struct{}
	allFailedOnce   sync.Once
}

func (f *fakeDeviceCode) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	f.mu.Lock()
	signedIn, silentErr := f.signedIn, f.silentErr
	f.mu.Unlock()
	if silentErr != nil {
		return azcore.AccessToken{}, silentErr
	}
	if !signedIn {
		if f.allFailed != nil && f.failures.Add(1) >= f.waitForFailures {
			f.allFailedOnce.Do(func() { close(f.allFailed) })
		}
		return azcore.AccessToken{}, &azidentity.AuthenticationRequiredError{}
	}
	return azcore.AccessToken{Token: "token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func (f *fakeDeviceCode) Authenticate(ctx context.Context, opts *policy.TokenRequestOptions) (azidentity.AuthenticationRecord, error) {
	if f.allFailed != nil {
		select {
		case <-f.allFailed:
		case <-ctx.Done():
			return azidentity.AuthenticationRecord{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCalls++
	if opts == nil || len(opts.Scopes) != 1 || opts.Scopes[0] != graphScope {
		return azidentity.AuthenticationRecord{}, errors.New("sign-in requested without the Graph .default scope")
	}
	if f.authErr != nil {
		return azidentity.AuthenticationRecord{}, f.authErr
	}
	f.signedIn = true
	return f.record, nil
}

func (f *fakeDeviceCode) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authCalls
}

var graphRequest = policy.TokenRequestOptions{Scopes: []string{graphScope}}

func TestCachedDeviceCodeCredential(t *testing.T) {
	t.Run("silent success never signs in", func(t *testing.T) {
		useConfigDir(t)
		inner := &fakeDeviceCode{signedIn: true}
		cred := &cachedDeviceCodeCredential{inner: inner, interactive: true, tenantID: "tenant-1", clientID: "client-1"}
		if _, err := cred.GetToken(context.Background(), graphRequest); err != nil {
			t.Fatalf("GetToken() error = %v", err)
		}
		if got := inner.calls(); got != 0 {
			t.Errorf("Authenticate called %d times, want 0 for a cached session", got)
		}
	})

	t.Run("authentication required signs in once, saves the record, retries", func(t *testing.T) {
		useConfigDir(t)
		inner := &fakeDeviceCode{record: testRecord("tenant-1", "client-1")}
		cred := &cachedDeviceCodeCredential{inner: inner, interactive: true, tenantID: "tenant-1", clientID: "client-1"}
		tk, err := cred.GetToken(context.Background(), graphRequest)
		if err != nil {
			t.Fatalf("GetToken() error = %v", err)
		}
		if tk.Token == "" {
			t.Error("GetToken() returned no token after the sign-in")
		}
		if got := inner.calls(); got != 1 {
			t.Errorf("Authenticate called %d times, want 1", got)
		}
		if got, ok := loadAuthRecord("tenant-1", "client-1"); !ok || got != inner.record {
			t.Errorf("record not saved after the sign-in: got %+v, found %v", got, ok)
		}
	})

	t.Run("non-interactive variant never signs in", func(t *testing.T) {
		useConfigDir(t)
		inner := &fakeDeviceCode{}
		cred := &cachedDeviceCodeCredential{inner: inner, interactive: false, tenantID: "tenant-1", clientID: "client-1"}
		_, err := cred.GetToken(context.Background(), graphRequest)
		var required *azidentity.AuthenticationRequiredError
		if !errors.As(err, &required) {
			t.Errorf("GetToken() error = %v, want the authentication-required error unchanged", err)
		}
		if got := inner.calls(); got != 0 {
			t.Errorf("Authenticate called %d times, want 0 on the non-interactive variant", got)
		}
	})

	t.Run("other errors pass through", func(t *testing.T) {
		useConfigDir(t)
		cause := errors.New("network unreachable")
		inner := &fakeDeviceCode{silentErr: cause}
		cred := &cachedDeviceCodeCredential{inner: inner, interactive: true, tenantID: "tenant-1", clientID: "client-1"}
		if _, err := cred.GetToken(context.Background(), graphRequest); !errors.Is(err, cause) {
			t.Errorf("GetToken() error = %v, want %v unchanged", err, cause)
		}
		if got := inner.calls(); got != 0 {
			t.Errorf("Authenticate called %d times, want 0 for a non-interaction error", got)
		}
	})

	t.Run("failed sign-in is returned", func(t *testing.T) {
		useConfigDir(t)
		cause := errors.New("user declined")
		inner := &fakeDeviceCode{authErr: cause}
		cred := &cachedDeviceCodeCredential{inner: inner, interactive: true, tenantID: "tenant-1", clientID: "client-1"}
		if _, err := cred.GetToken(context.Background(), graphRequest); !errors.Is(err, cause) {
			t.Errorf("GetToken() error = %v, want it to wrap %v", err, cause)
		}
		if _, ok := loadAuthRecord("tenant-1", "client-1"); ok {
			t.Error("a record was saved although the sign-in failed")
		}
	})
}

// TestCachedDeviceCodeCredentialConcurrentSignIn guards the single prompt: the
// pipeline's workers request tokens concurrently, and on a fresh session they
// must share one device-code sign-in instead of each starting their own.
func TestCachedDeviceCodeCredentialConcurrentSignIn(t *testing.T) {
	useConfigDir(t)
	const callers = 8
	inner := &fakeDeviceCode{
		record:          testRecord("tenant-1", "client-1"),
		// Every caller's silent attempt, plus the first lock holder's re-check.
		waitForFailures: callers + 1,
		allFailed:       make(chan struct{}),
	}
	cred := &cachedDeviceCodeCredential{inner: inner, interactive: true, tenantID: "tenant-1", clientID: "client-1"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cred.GetToken(ctx, graphRequest); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("GetToken() error = %v", err)
	}
	if got := inner.calls(); got != 1 {
		t.Errorf("Authenticate called %d times for %d concurrent callers, want exactly 1", got, callers)
	}
}

// TestNewCredentialTokenCache guards how the dedicated-app credential is built
// around the persistent cache: with a cache it is the cached session; without
// one it falls back to the plain device-code credential and warns once.
func TestNewCredentialTokenCache(t *testing.T) {
	t.Run("cache unavailable falls back and warns once", func(t *testing.T) {
		useConfigDir(t)
		calls := useCacheFactory(t, errors.New("no keychain"))
		buf := captureLog(t)

		for range 2 {
			cred, err := NewCredential("client-1", "tenant-1")
			if err != nil {
				t.Fatalf("NewCredential() error = %v", err)
			}
			if _, ok := cred.(*azidentity.DeviceCodeCredential); !ok {
				t.Errorf("NewCredential() = %T, want the plain device-code credential", cred)
			}
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("cache factory called %d times, want 1", got)
		}
		if got := strings.Count(buf.String(), "Token cache unavailable: no keychain; signing in on every run"); got != 1 {
			t.Errorf("warning printed %d times, want once; log:\n%s", got, buf.String())
		}
	})

	t.Run("cache available yields the cached session", func(t *testing.T) {
		useConfigDir(t)
		useCacheFactory(t, nil)

		cred, err := NewCredential("client-1", "tenant-1")
		if err != nil {
			t.Fatalf("NewCredential() error = %v", err)
		}
		cached, ok := cred.(*cachedDeviceCodeCredential)
		if !ok {
			t.Fatalf("NewCredential() = %T, want the cached device-code session", cred)
		}
		if !cached.interactive {
			t.Error("NewCredential() built a non-interactive session")
		}

		cred, err = NewNonInteractiveCredential("client-1", "tenant-1")
		if err != nil {
			t.Fatalf("NewNonInteractiveCredential() error = %v", err)
		}
		if cached, ok := cred.(*cachedDeviceCodeCredential); !ok || cached.interactive {
			t.Errorf("NewNonInteractiveCredential() = %T (interactive=%v), want a non-interactive cached session", cred, ok && cached.interactive)
		}
	})

	t.Run("az login path never builds the cache", func(t *testing.T) {
		calls := useCacheFactory(t, nil)
		if _, err := NewCredential("", ""); err != nil {
			t.Fatalf("NewCredential() error = %v", err)
		}
		if got := calls.Load(); got != 0 {
			t.Errorf("cache factory called %d times on the az login path, want 0", got)
		}
	})
}

// TestNewCredentialTenantRequired guards the error a profile with a client-id
// but no tenant-id produces: it names the profile keys, never the removed flags
// or environment variables.
func TestNewCredentialTenantRequired(t *testing.T) {
	_, err := NewCredential("client-1", "")
	if !errors.Is(err, ErrTenantIDRequired) {
		t.Fatalf("NewCredential() error = %v, want ErrTenantIDRequired", err)
	}
	if got, want := err.Error(), "tenant-id is required when client-id is set in the tenant profile"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	for _, stale := range []string{"--client-id", "--tenant-id", "AZURE_RD_"} {
		if strings.Contains(err.Error(), stale) {
			t.Errorf("error %q still names %q", err, stale)
		}
	}
}

func TestTokenCacheStatus(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		useConfigDir(t)
		useCacheFactory(t, errors.New("no keychain"))
		captureLog(t)
		if got, want := TokenCacheStatus("tenant-1", "client-1"), "unavailable (no keychain)"; got != want {
			t.Errorf("TokenCacheStatus() = %q, want %q", got, want)
		}
	})

	t.Run("no session yet", func(t *testing.T) {
		useConfigDir(t)
		useCacheFactory(t, nil)
		if got, want := TokenCacheStatus("tenant-1", "client-1"), "no session yet (the next run signs in once)"; got != want {
			t.Errorf("TokenCacheStatus() = %q, want %q", got, want)
		}
	})

	t.Run("active", func(t *testing.T) {
		useConfigDir(t)
		useCacheFactory(t, nil)
		if err := saveAuthRecord("tenant-1", "client-1", testRecord("tenant-1", "client-1")); err != nil {
			t.Fatal(err)
		}
		path, _ := authRecordPath("tenant-1", "client-1")
		stamp := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		if got, want := TokenCacheStatus("tenant-1", "client-1"), "active (record from 2026-09-30)"; got != want {
			t.Errorf("TokenCacheStatus() = %q, want %q", got, want)
		}
	})
}
