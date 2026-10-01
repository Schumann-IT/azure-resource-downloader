package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
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
	var claims struct {
		UPN               string `json:"upn"`
		PreferredUsername string `json:"preferred_username"`
		UniqueName        string `json:"unique_name"`
		Email             string `json:"email"`
		TenantID          string `json:"tid"`
		ObjectID          string `json:"oid"`
	}
	if err := decodeClaims(token, &claims); err != nil {
		return Identity{}, err
	}

	return Identity{
		Username: firstNonEmpty(claims.UPN, claims.PreferredUsername, claims.UniqueName, claims.Email),
		TenantID: claims.TenantID,
		ObjectID: claims.ObjectID,
	}, nil
}

// TokenClaims are the claims of a Microsoft Graph access token that say which
// application the token was issued to and which delegated permissions it
// carries. They are read for display only; the token itself is never logged.
type TokenClaims struct {
	// AppID is the application (client) id the token was issued to (the
	// "appid" claim): the Azure CLI's first-party app, or the dedicated app.
	AppID string
	// AppDisplayName is that application's display name ("app_displayname").
	AppDisplayName string
	// Scopes are the delegated permissions the token carries (the
	// space-separated "scp" claim), sorted and free of duplicates.
	Scopes []string
}

// GraphTokenClaims requests a Microsoft Graph token from cred and decodes its
// application and scope claims, without any directory call.
func GraphTokenClaims(ctx context.Context, cred azcore.TokenCredential) (TokenClaims, error) {
	tk, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{graphScope}})
	if err != nil {
		return TokenClaims{}, fmt.Errorf("failed to acquire a Microsoft Graph token: %w", err)
	}
	return parseTokenClaims(tk.Token)
}

// parseTokenClaims decodes the "appid", "app_displayname" and "scp" claims of
// a JWT access token. Like parseIdentityClaims it does not verify the
// signature: the token was just issued to this process and is only read.
func parseTokenClaims(token string) (TokenClaims, error) {
	var claims struct {
		AppID          string `json:"appid"`
		AppDisplayName string `json:"app_displayname"`
		Scope          string `json:"scp"`
	}
	if err := decodeClaims(token, &claims); err != nil {
		return TokenClaims{}, err
	}

	seen := map[string]bool{}
	var scopes []string
	for _, scope := range strings.Fields(claims.Scope) {
		if !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)

	return TokenClaims{AppID: claims.AppID, AppDisplayName: claims.AppDisplayName, Scopes: scopes}, nil
}

// decodeClaims decodes the claims segment of a JWT access token into v.
func decodeClaims(token string, v any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("access token is not a JWT (expected 3 segments, got %d)", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("failed to decode token claims: %w", err)
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return fmt.Errorf("failed to parse token claims: %w", err)
	}
	return nil
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
