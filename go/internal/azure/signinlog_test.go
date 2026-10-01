package azure

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// scriptedCredential answers token requests from a switchable error, so a
// test can fail the first requests and succeed later ones.
type scriptedCredential struct {
	mu    sync.Mutex
	err   error
	token string
	calls int
}

func (c *scriptedCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.err != nil {
		return azcore.AccessToken{}, c.err
	}
	return azcore.AccessToken{Token: c.token}, nil
}

func (c *scriptedCredential) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func TestWithSignInLog(t *testing.T) {
	var buf bytes.Buffer
	logger.Default.SetOutput(&buf)
	t.Cleanup(func() { logger.Default.SetOutput(os.Stderr) })

	token := makeJWT(`{"upn":"alice@contoso.com","tid":"tenant-1","oid":"object-1"}`)
	inner := &scriptedCredential{err: errors.New("device-code sign-in failed"), token: token}
	cred := WithSignInLog(inner)
	opts := policy.TokenRequestOptions{Scopes: []string{graphScope}}

	// Failures log nothing and pass through unchanged.
	if _, err := cred.GetToken(context.Background(), opts); err == nil {
		t.Fatal("GetToken() succeeded, want the inner error")
	}
	if strings.Contains(buf.String(), "Signed in") {
		t.Fatalf("logged a sign-in after a failed token request:\n%s", buf.String())
	}

	// A later success, by many concurrent callers, logs exactly once.
	inner.setErr(nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := cred.GetToken(context.Background(), opts)
			if err != nil || tk.Token != token {
				t.Errorf("GetToken() = (%q, %v), want the inner token", tk.Token, err)
			}
		}()
	}
	wg.Wait()

	out := buf.String()
	if n := strings.Count(out, "Signed in"); n != 1 {
		t.Errorf("\"Signed in\" logged %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "alice@contoso.com") || !strings.Contains(out, "tenant-1") {
		t.Errorf("the line lacks the account or tenant:\n%s", out)
	}
	if strings.Contains(out, token) {
		t.Error("the token was logged")
	}
	if inner.calls != 17 {
		t.Errorf("inner credential called %d times, want 17 (no extra request)", inner.calls)
	}
}

func TestWithSignInLogUnreadableToken(t *testing.T) {
	var buf bytes.Buffer
	logger.Default.SetOutput(&buf)
	t.Cleanup(func() { logger.Default.SetOutput(os.Stderr) })

	cred := WithSignInLog(&scriptedCredential{token: "opaque"})
	if _, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "Signed in"); n != 1 {
		t.Errorf("\"Signed in\" logged %d times, want 1:\n%s", n, buf.String())
	}
	if strings.Contains(buf.String(), "opaque") {
		t.Error("the token was logged")
	}
}
