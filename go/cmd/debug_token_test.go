package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"azure-resource-downloader/internal/azure"

	"github.com/charmbracelet/log"
)

// TestBuildGraphTokenReport guards the --debug Graph token section: every
// declared permission of every selected dedicated-app type appears once,
// marked covered or missing, sorted by type and permission whatever order the
// requirements map yields.
func TestBuildGraphTokenReport(t *testing.T) {
	claims := azure.TokenClaims{
		AppID:          "app-1",
		AppDisplayName: "Microsoft Azure CLI",
		Scopes:         []string{"User.Read", "DeviceManagementApps.ReadWrite.All"},
	}
	requirements := map[string][]string{
		"Microsoft.Graph/deviceAppManagement/mobileApps": {"DeviceManagementApps.Read.All"},
		"Microsoft.Graph/conditionalAccessPolicies":      {"Policy.Read.All", "Application.Read.All"},
	}

	got := buildGraphTokenReport(claims, requirements)

	want := graphTokenReport{
		AppID:          "app-1",
		AppDisplayName: "Microsoft Azure CLI",
		Scopes:         []string{"DeviceManagementApps.ReadWrite.All", "User.Read"},
		Permissions: []permissionStatus{
			{Type: "Microsoft.Graph/conditionalAccessPolicies", Permission: "Application.Read.All"},
			{Type: "Microsoft.Graph/conditionalAccessPolicies", Permission: "Policy.Read.All"},
			{Type: "Microsoft.Graph/deviceAppManagement/mobileApps", Permission: "DeviceManagementApps.Read.All", Covered: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildGraphTokenReport() =\n%+v\nwant\n%+v", got, want)
	}
	if covered, missing := got.counts(); covered != 1 || missing != 2 {
		t.Errorf("counts() = %d covered, %d missing, want 1 and 2", covered, missing)
	}
}

func TestLogGraphTokenReport(t *testing.T) {
	var buf bytes.Buffer
	logger := log.NewWithOptions(&buf, log.Options{})

	logGraphTokenReport(logger, buildGraphTokenReport(
		azure.TokenClaims{AppID: "app-1", Scopes: []string{"Policy.Read.All"}},
		map[string][]string{"Microsoft.Graph/conditionalAccessPolicies": {"Policy.Read.All", "Application.Read.All"}},
	))
	out := buf.String()
	for _, want := range []string{
		"Graph token", "app_id=app-1", "app_display_name=<none>", "scp=Policy.Read.All",
		"permission=Application.Read.All status=missing",
		"permission=Policy.Read.All status=covered",
		"covered=1 missing=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("section lacks %q; output:\n%s", want, out)
		}
	}

	buf.Reset()
	logGraphTokenReport(logger, buildGraphTokenReport(azure.TokenClaims{AppID: "app-1"}, nil))
	if !strings.Contains(buf.String(), "no selected type needs a dedicated app") {
		t.Errorf("empty selection not reported; output:\n%s", buf.String())
	}
}
