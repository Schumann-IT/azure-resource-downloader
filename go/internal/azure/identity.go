package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// Identity describes the signed-in principal, derived from the claims embedded
// in an access token rather than from a directory lookup.
type Identity struct {
	// Username is the user principal name / preferred username of the signed-in
	// identity (e.g. "alice@contoso.onmicrosoft.com").
	Username string
	// TenantID is the Entra tenant the token was issued for (the "tid" claim).
	TenantID string
	// ObjectID is the signed-in principal's directory object ID (the "oid" claim).
	ObjectID string
}

// armScope and graphScope are the resource scopes an access token is requested
// for. ARM is tried first (the tool's primary surface); Graph is the fallback
// for a dedicated app registration that carries only Microsoft Graph scopes.
const (
	armScope   = "https://management.azure.com/.default"
	graphScope = "https://graph.microsoft.com/.default"
)

// SignedInIdentity acquires an access token from cred and extracts the
// signed-in principal's claims from it, without making any directory call. It
// requests an ARM-scoped token first and falls back to a Microsoft Graph scope
// so it also works for a dedicated app registration that lacks ARM permissions.
func SignedInIdentity(ctx context.Context, cred azcore.TokenCredential) (Identity, error) {
	var lastErr error
	for _, scope := range []string{armScope, graphScope} {
		tk, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
		if err != nil {
			lastErr = err
			continue
		}
		return parseIdentityClaims(tk.Token)
	}
	return Identity{}, fmt.Errorf("failed to acquire an access token: %w", lastErr)
}

// parseIdentityClaims decodes the claims segment of a JWT access token and maps
// the recognised name/tenant/object claims onto an Identity. It does not verify
// the token signature: the token was just issued to this process by the
// credential and is only being read for display.
func parseIdentityClaims(token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, fmt.Errorf("access token is not a JWT (expected 3 segments, got %d)", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, fmt.Errorf("failed to decode token claims: %w", err)
	}

	var claims struct {
		UPN               string `json:"upn"`
		PreferredUsername string `json:"preferred_username"`
		UniqueName        string `json:"unique_name"`
		Email             string `json:"email"`
		TenantID          string `json:"tid"`
		ObjectID          string `json:"oid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, fmt.Errorf("failed to parse token claims: %w", err)
	}

	return Identity{
		Username: firstNonEmpty(claims.UPN, claims.PreferredUsername, claims.UniqueName, claims.Email),
		TenantID: claims.TenantID,
		ObjectID: claims.ObjectID,
	}, nil
}

// firstNonEmpty returns the first non-empty string in vals, or "" if none is set.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
