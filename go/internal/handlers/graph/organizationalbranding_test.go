package graph

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"azure-resource-downloader/internal/azure"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
	betaodataerrors "github.com/microsoftgraph/msgraph-beta-sdk-go/models/odataerrors"
)

func TestOrganizationalBrandingHandler_GetType(t *testing.T) {
	handler, err := NewOrganizationalBrandingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewOrganizationalBrandingHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/organizationalBranding"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestOrganizationalBrandingHandler_Transform(t *testing.T) {
	handler, err := NewOrganizationalBrandingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewOrganizationalBrandingHandler() unexpected error: %v", err)
	}

	branding := betamodels.NewOrganizationalBranding()
	id := "0"
	branding.SetId(&id)

	transformed, err := handler.Transform(branding)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != organizationalBrandingFallbackName {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, organizationalBrandingFallbackName)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/organizationalBranding" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/organizationalBranding")
	}
}

// newBrandingODataError builds the beta OData error Graph returns for the
// branding endpoint.
func newBrandingODataError(status int, code, message string) *betaodataerrors.ODataError {
	main := betaodataerrors.NewMainError()
	main.SetCode(&code)
	main.SetMessage(&message)
	e := betaodataerrors.NewODataError()
	e.SetErrorEscaped(main)
	e.SetStatusCode(status)
	return e
}

func TestBrandingListIDs(t *testing.T) {
	withID := betamodels.NewOrganizationalBranding()
	id := "0"
	withID.SetId(&id)

	tests := []struct {
		name        string
		branding    betamodels.OrganizationalBrandingable
		err         error
		wantIDs     []string
		wantSummary []string // substrings of azure.ErrorSummary(err); nil = no error expected
	}{
		{
			name: "404 means no default branding: lists as empty",
			err:  newBrandingODataError(http.StatusNotFound, "Request_ResourceNotFound", "Resource 'org' does not exist or one of its queried reference-property objects are not present."),
		},
		{
			name:        "403 fails the listing with status and hint",
			err:         newBrandingODataError(http.StatusForbidden, "Authorization_RequestDenied", "Insufficient privileges to complete the operation."),
			wantSummary: []string{"HTTP 403", "Authorization_RequestDenied", "failed to get organizational branding", "(hint: requires 'OrganizationalBranding.Read.All'"},
		},
		{
			name: "nil body lists as empty",
		},
		{
			name:     "body with an ID lists that ID",
			branding: withID,
			wantIDs:  []string{"0"},
		},
		{
			name:     "body without an ID lists the pseudo-ID",
			branding: betamodels.NewOrganizationalBranding(),
			wantIDs:  []string{"organizationalBranding"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := brandingListIDs(tt.branding, tt.err)
			if tt.wantSummary == nil {
				if err != nil {
					t.Fatalf("brandingListIDs() unexpected error: %v", err)
				}
				if !reflect.DeepEqual(ids, tt.wantIDs) {
					t.Errorf("brandingListIDs() = %v, want %v", ids, tt.wantIDs)
				}
				return
			}
			if err == nil {
				t.Fatalf("brandingListIDs() = %v, nil; want an error", ids)
			}
			if ids != nil {
				t.Errorf("brandingListIDs() ids = %v, want nil on error", ids)
			}
			summary := azure.ErrorSummary(err)
			for _, want := range tt.wantSummary {
				if !strings.Contains(summary, want) {
					t.Errorf("ErrorSummary() = %q, missing %q", summary, want)
				}
			}
		})
	}
}
