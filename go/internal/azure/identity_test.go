package azure

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// makeJWT builds a syntactically valid JWT (header.payload.signature) whose
// claims segment is the given raw JSON. The header and signature are dummies:
// parseIdentityClaims never verifies them.
func makeJWT(claimsJSON string) string {
	seg := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	return seg(`{"alg":"none","typ":"JWT"}`) + "." + seg(claimsJSON) + ".sig"
}

func TestParseIdentityClaims(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    Identity
		wantErr bool
	}{
		{
			name:  "upn preferred over other name claims",
			token: makeJWT(`{"upn":"alice@contoso.com","preferred_username":"pref@contoso.com","unique_name":"uniq@contoso.com","tid":"tenant-1","oid":"object-1"}`),
			want:  Identity{Username: "alice@contoso.com", TenantID: "tenant-1", ObjectID: "object-1"},
		},
		{
			name:  "falls back to preferred_username",
			token: makeJWT(`{"preferred_username":"pref@contoso.com","tid":"tenant-2","oid":"object-2"}`),
			want:  Identity{Username: "pref@contoso.com", TenantID: "tenant-2", ObjectID: "object-2"},
		},
		{
			name:  "falls back to unique_name then email",
			token: makeJWT(`{"email":"mail@contoso.com","tid":"tenant-3"}`),
			want:  Identity{Username: "mail@contoso.com", TenantID: "tenant-3"},
		},
		{
			name:  "no name claims yields empty username",
			token: makeJWT(`{"tid":"tenant-4","oid":"object-4"}`),
			want:  Identity{Username: "", TenantID: "tenant-4", ObjectID: "object-4"},
		},
		{
			name:    "not a JWT",
			token:   "opaque-token",
			wantErr: true,
		},
		{
			name:    "claims segment not valid base64",
			token:   "header.!!!not-base64!!!.sig",
			wantErr: true,
		},
		{
			name:    "claims segment not valid JSON",
			token:   makeJWT(`{not json`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIdentityClaims(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil (identity=%+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseIdentityClaims() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeCredential is a policy.TokenCredential test double: it returns tokenByScope
// for the requested scope, or errByScope, without any network call.
type fakeCredential struct {
	tokenByScope map[string]string
	errByScope   map[string]error
}

func (f fakeCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if len(opts.Scopes) == 0 {
		return azcore.AccessToken{}, errors.New("no scope requested")
	}
	scope := opts.Scopes[0]
	if err := f.errByScope[scope]; err != nil {
		return azcore.AccessToken{}, err
	}
	if tok, ok := f.tokenByScope[scope]; ok {
		return azcore.AccessToken{Token: tok, ExpiresOn: time.Now().Add(time.Hour)}, nil
	}
	return azcore.AccessToken{}, errors.New("scope not configured")
}

func TestSignedInIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("uses ARM-scoped token when available", func(t *testing.T) {
		cred := fakeCredential{tokenByScope: map[string]string{
			armScope: makeJWT(`{"upn":"arm@contoso.com","tid":"t","oid":"o"}`),
		}}
		got, err := SignedInIdentity(ctx, cred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Username != "arm@contoso.com" {
			t.Errorf("username = %q, want arm@contoso.com", got.Username)
		}
	})

	t.Run("falls back to Graph scope when ARM is denied", func(t *testing.T) {
		cred := fakeCredential{
			errByScope:   map[string]error{armScope: errors.New("AADSTS65001: no ARM consent")},
			tokenByScope: map[string]string{graphScope: makeJWT(`{"upn":"graph@contoso.com","tid":"t","oid":"o"}`)},
		}
		got, err := SignedInIdentity(ctx, cred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Username != "graph@contoso.com" {
			t.Errorf("username = %q, want graph@contoso.com", got.Username)
		}
	})

	t.Run("errors when no scope yields a token", func(t *testing.T) {
		cred := fakeCredential{errByScope: map[string]error{
			armScope:   errors.New("denied"),
			graphScope: errors.New("denied"),
		}}
		if _, err := SignedInIdentity(ctx, cred); err == nil {
			t.Fatal("expected an error when every scope is denied, got nil")
		}
	})
}

func TestParseTokenClaims(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    TokenClaims
		wantErr bool
	}{
		{
			name:  "app and scopes, sorted and de-duplicated",
			token: makeJWT(`{"appid":"04b07795-8ddb-461a-bbee-02f9e1bf7b46","app_displayname":"Microsoft Azure CLI","scp":"User.Read Policy.Read.All  Group.Read.All User.Read"}`),
			want: TokenClaims{
				AppID:          "04b07795-8ddb-461a-bbee-02f9e1bf7b46",
				AppDisplayName: "Microsoft Azure CLI",
				Scopes:         []string{"Group.Read.All", "Policy.Read.All", "User.Read"},
			},
		},
		{
			name:  "no scp claim",
			token: makeJWT(`{"appid":"app-1"}`),
			want:  TokenClaims{AppID: "app-1"},
		},
		{
			name:    "not a JWT",
			token:   "opaque-token",
			wantErr: true,
		},
		{
			name:    "claims segment not valid JSON",
			token:   makeJWT(`{not json`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTokenClaims(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got nil (claims=%+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseTokenClaims() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGraphTokenClaims(t *testing.T) {
	ctx := context.Background()

	t.Run("requests the Graph scope", func(t *testing.T) {
		cred := fakeCredential{tokenByScope: map[string]string{
			graphScope: makeJWT(`{"appid":"app-1","scp":"User.Read"}`),
		}}
		got, err := GraphTokenClaims(ctx, cred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.AppID != "app-1" || !reflect.DeepEqual(got.Scopes, []string{"User.Read"}) {
			t.Errorf("GraphTokenClaims() = %+v", got)
		}
	})

	t.Run("token failure is returned", func(t *testing.T) {
		cause := errors.New("denied")
		cred := fakeCredential{errByScope: map[string]error{graphScope: cause}}
		if _, err := GraphTokenClaims(ctx, cred); !errors.Is(err, cause) {
			t.Errorf("GraphTokenClaims() error = %v, want it to wrap %v", err, cause)
		}
	})
}
