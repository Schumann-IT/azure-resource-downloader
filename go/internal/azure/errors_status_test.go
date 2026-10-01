package azure

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	betaodataerrors "github.com/microsoftgraph/msgraph-beta-sdk-go/models/odataerrors"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

const brandingHint = "(hint: requires 'OrganizationalBranding.Read.All' permission in Microsoft Graph)"

// newV1ODataError builds a Microsoft Graph v1.0 OData error the way the SDK
// deserialises one: a main error with code and message, and the HTTP status.
func newV1ODataError(status int, code, message string) *odataerrors.ODataError {
	main := odataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&message)
	e := odataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(status)
	return e
}

// newBetaODataError is newV1ODataError for the beta SDK.
func newBetaODataError(status int, code, message string) *betaodataerrors.ODataError {
	main := betaodataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&message)
	e := betaodataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(status)
	return e
}

// newARMError builds an azcore response error with a request line, as ARM
// clients return them.
func newARMError(status int, code string) *azcore.ResponseError {
	return &azcore.ResponseError{
		StatusCode: status,
		ErrorCode:  code,
		RawResponse: &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Body:       http.NoBody,
			Request: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Scheme: "https", Host: "management.azure.com", Path: "/subscriptions/x/resources"},
			},
		},
	}
}

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantOK     bool
	}{
		{name: "nil", err: nil},
		{name: "untyped", err: errors.New("connection reset by peer")},
		{name: "v1.0 OData 404", err: newV1ODataError(http.StatusNotFound, "Request_ResourceNotFound", "gone"), wantStatus: 404, wantOK: true},
		{name: "beta OData 403 wrapped", err: fmt.Errorf("failed: %w", newBetaODataError(http.StatusForbidden, "Authorization_RequestDenied", "denied")), wantStatus: 403, wantOK: true},
		{name: "azcore 404", err: newARMError(http.StatusNotFound, "ResourceNotFound"), wantStatus: 404, wantOK: true},
		{name: "azcore 403 wrapped", err: fmt.Errorf("failed: %w", newARMError(http.StatusForbidden, "AuthorizationFailed")), wantStatus: 403, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, ok := HTTPStatus(tt.err)
			if status != tt.wantStatus || ok != tt.wantOK {
				t.Errorf("HTTPStatus() = (%d, %v), want (%d, %v)", status, ok, tt.wantStatus, tt.wantOK)
			}
		})
	}
}

func TestGraphErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "untyped", err: errors.New("boom"), want: ""},
		{name: "azcore carries no Graph code", err: newARMError(http.StatusForbidden, "AuthorizationFailed"), want: ""},
		{name: "v1.0", err: newV1ODataError(http.StatusNotFound, "Request_ResourceNotFound", "gone"), want: "Request_ResourceNotFound"},
		{name: "beta wrapped", err: fmt.Errorf("x: %w", newBetaODataError(http.StatusForbidden, "Authorization_RequestDenied", "denied")), want: "Authorization_RequestDenied"},
		{name: "OData error without main error", err: odataerrors.NewODataError(), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GraphErrorCode(tt.err); got != tt.want {
				t.Errorf("GraphErrorCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsPermissionErrorTyped(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "Graph v1.0 403", err: newV1ODataError(http.StatusForbidden, "Authorization_RequestDenied", "Access denied"), want: true},
		{name: "Graph beta 401 wrapped", err: fmt.Errorf("x: %w", newBetaODataError(http.StatusUnauthorized, "InvalidAuthenticationToken", "token expired")), want: true},
		{name: "Graph beta 404", err: newBetaODataError(http.StatusNotFound, "Request_ResourceNotFound", "Resource does not exist"), want: false},
		{name: "azcore 404", err: newARMError(http.StatusNotFound, "ResourceNotFound"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPermissionError(tt.err); got != tt.want {
				t.Errorf("IsPermissionError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorSummaryTyped(t *testing.T) {
	notFound := "Resource 'org-1' does not exist or one of its queried reference-property objects are not present."
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "v1.0 404 keeps context, drops the hint",
			err:  fmt.Errorf("failed to get organizational branding: %w %s", newV1ODataError(http.StatusNotFound, "Request_ResourceNotFound", notFound), brandingHint),
			want: "HTTP 404 Request_ResourceNotFound: failed to get organizational branding: " + notFound,
		},
		{
			name: "beta 404 keeps context, drops the hint",
			err:  fmt.Errorf("failed to get organizational branding: %w %s", newBetaODataError(http.StatusNotFound, "Request_ResourceNotFound", notFound), brandingHint),
			want: "HTTP 404 Request_ResourceNotFound: failed to get organizational branding: " + notFound,
		},
		{
			name: "v1.0 403 keeps the hint",
			err:  fmt.Errorf("failed to get organizational branding: %w %s", newV1ODataError(http.StatusForbidden, "Authorization_RequestDenied", "Insufficient privileges to complete the operation."), brandingHint),
			want: "HTTP 403 Authorization_RequestDenied: failed to get organizational branding: Insufficient privileges to complete the operation. " + brandingHint,
		},
		{
			name: "beta 403 keeps the hint",
			err:  fmt.Errorf("failed to get organizational branding: %w %s", newBetaODataError(http.StatusForbidden, "Authorization_RequestDenied", "Insufficient privileges to complete the operation."), brandingHint),
			want: "HTTP 403 Authorization_RequestDenied: failed to get organizational branding: Insufficient privileges to complete the operation. " + brandingHint,
		},
		{
			name: "azcore 404 without hint",
			err:  fmt.Errorf("failed to list resources: %w (hint: requires 'Reader' role)", newARMError(http.StatusNotFound, "ResourceNotFound")),
			want: "HTTP 404 ResourceNotFound: failed to list resources: GET https://management.azure.com/subscriptions/x/resources",
		},
		{
			name: "azcore 403 with hint",
			err:  fmt.Errorf("failed to list resources: %w (hint: requires 'Reader' role)", newARMError(http.StatusForbidden, "AuthorizationFailed")),
			want: "HTTP 403 AuthorizationFailed: failed to list resources: GET https://management.azure.com/subscriptions/x/resources (hint: requires 'Reader' role)",
		},
		{
			name: "untyped permission error keeps the hint",
			err:  errors.New("failed to list policies: Request Authorization failed " + brandingHint),
			want: "failed to list policies: Request Authorization failed " + brandingHint,
		},
		{
			name: "untyped non-permission error drops the hint",
			err:  errors.New("failed to list policies: connection reset by peer " + brandingHint),
			want: "failed to list policies: connection reset by peer",
		},
		{
			name: "untyped multi-line permission error surfaces the JSON message and hint",
			err:  errors.New("failed to list device configurations: {\r\n  \"Message\": \"Application is not authorized to perform this operation\"\r\n} " + brandingHint),
			want: "failed to list device configurations: Application is not authorized to perform this operation " + brandingHint,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ErrorSummary(tt.err)
			if got != tt.want {
				t.Errorf("ErrorSummary() = %q, want %q", got, tt.want)
			}
			if again := ErrorSummary(tt.err); again != got {
				t.Errorf("ErrorSummary() not deterministic: %q then %q", got, again)
			}
		})
	}
}
