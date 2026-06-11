package azure

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "ARM AuthorizationFailed",
			err:  errors.New("RESPONSE 403: 403 Forbidden\nERROR CODE: AuthorizationFailed"),
			want: true,
		},
		{
			name: "ARM does not have authorization",
			err:  errors.New("The client '...' does not have authorization to perform action 'Microsoft.Resources/subscriptions/resources/read'"),
			want: true,
		},
		{
			name: "Graph missing scopes",
			err:  errors.New("failed to list conditional access policies: You cannot perform the requested operation, required scopes are missing in the token."),
			want: true,
		},
		{
			name: "Graph request authorization failed",
			err:  errors.New("failed to list authentication strength policies: Request Authorization failed"),
			want: true,
		},
		{
			name: "Intune forbidden",
			err:  errors.New(`{"ErrorCode":"Forbidden","Message":"..."}`),
			want: true,
		},
		{
			name: "Intune missing DeviceManagementConfiguration scope",
			err:  errors.New(`failed to list device configurations: {"Message":"Application is not authorized to perform this operation. Application must have one of the following scopes: DeviceManagementConfiguration.Read.All, DeviceManagementConfiguration.ReadWrite.All"}`),
			want: true,
		},
		{
			name: "wrapped permission error",
			err:  fmt.Errorf("failed to fetch resource: %w", errors.New("AuthorizationFailed")),
			want: true,
		},
		{
			name: "non-permission error",
			err:  errors.New("connection reset by peer"),
			want: false,
		},
		{
			name: "rate limited is not a permission error",
			err:  errors.New("429 TooManyRequests"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPermissionError(tt.err); got != tt.want {
				t.Errorf("IsPermissionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestErrorSummary(t *testing.T) {
	armErr := &azcore.ResponseError{
		StatusCode: http.StatusForbidden,
		ErrorCode:  "AuthorizationFailed",
		RawResponse: &http.Response{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Body:       http.NoBody,
			Request: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Scheme: "https", Host: "management.azure.com", Path: "/subscriptions/x/resources"},
			},
		},
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "single-line error is returned as-is",
			err:  errors.New("connection reset by peer"),
			want: "connection reset by peer",
		},
		{
			name: "ARM response error collapses to status and code",
			err:  armErr,
			want: "HTTP 403 AuthorizationFailed",
		},
		{
			name: "wrapped ARM response error",
			err:  fmt.Errorf("failed to list resources: %w", armErr),
			want: "HTTP 403 AuthorizationFailed",
		},
		{
			name: "multiline Intune error surfaces embedded JSON Message and hint",
			err:  errors.New("failed to list device configurations: {\r\n  \"Message\": \"Application is not authorized\"\r\n} (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)"),
			want: "failed to list device configurations: Application is not authorized (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)",
		},
		{
			name: "multiline error without JSON Message keeps first line and hint",
			err:  errors.New("failed to list device configurations: {\r\n  \"_version\": 3\r\n} (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)"),
			want: "failed to list device configurations (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)",
		},
		{
			name: "single-line error with hint is unchanged",
			err:  errors.New("failed to list configuration policies: Request Authorization failed (hint: requires 'Policy.Read.All' permission in Microsoft Graph)"),
			want: "failed to list configuration policies: Request Authorization failed (hint: requires 'Policy.Read.All' permission in Microsoft Graph)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorSummary(tt.err); got != tt.want {
				t.Errorf("ErrorSummary(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
