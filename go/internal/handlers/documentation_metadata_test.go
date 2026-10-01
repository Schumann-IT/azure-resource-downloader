package handlers

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

// learnPrefix is the only origin a Microsoft Learn reference may have.
const learnPrefix = "https://learn.microsoft.com/en-us/"

// adminCenterHosts are the portals an AdminCenter deep link may point to.
var adminCenterHosts = map[string]bool{
	"intune.microsoft.com": true,
	"entra.microsoft.com":  true,
	"portal.azure.com":     true,
}

var (
	// azureTypeLiteral finds the type a Graph handler file registers.
	azureTypeLiteral = regexp.MustCompile(`azureType:\s*"([^"]+)"`)
	// graphAPILink finds a Graph REST API reference link in a source file.
	graphAPILink = regexp.MustCompile(`"(https://learn\.microsoft\.com/en-us/graph/api/[^"]+)"`)
	// requiresHint finds a scope named in a "hint: requires '…'" error text.
	requiresHint = regexp.MustCompile(`requires '([^']+)'`)
)

// TestHandlerDocumentationMetadata checks every registered handler's
// documentation metadata: it is exposed through models.Documented, carries the
// read permission, an API reference and a permissions page, and every link is
// on an allowed origin and no longer uses the retired Intune documentation
// path.
func TestHandlerDocumentationMetadata(t *testing.T) {
	registry := NewRegistry(stubCredential{}, "sub-123", false)

	for _, resourceType := range registry.GetAllTypes() {
		t.Run(resourceType, func(t *testing.T) {
			handler, err := registry.Get(resourceType)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", resourceType, err)
			}
			documented, ok := handler.(models.Documented)
			if !ok {
				t.Fatalf("handler for %q does not implement models.Documented", resourceType)
			}
			doc := documented.Documentation()
			if doc.AzureType != resourceType {
				t.Errorf("Documentation().AzureType = %q, want %q", doc.AzureType, resourceType)
			}

			if doc.Links.EndpointDocs == "" {
				t.Error("Links.EndpointDocs is empty")
			}
			if len(doc.RequiredPermissions) == 0 {
				t.Error("RequiredPermissions is empty")
			}
			if doc.Links.Permissions == "" {
				t.Error("Links.Permissions is empty")
			}

			learnLinks := map[string]string{
				"EndpointDocs":    doc.Links.EndpointDocs,
				"SchemaReference": doc.Links.SchemaReference,
				"Permissions":     doc.Links.Permissions,
			}
			for i, link := range doc.Links.BestPractices {
				learnLinks[fmt.Sprintf("BestPractices[%d]", i)] = link
			}
			for field, link := range learnLinks {
				if link == "" {
					continue
				}
				if !strings.HasPrefix(link, learnPrefix) {
					t.Errorf("%s = %q, want a %s… link", field, link, learnPrefix)
				}
			}

			if admin := doc.Links.AdminCenter; admin != "" {
				u, err := url.Parse(admin)
				if err != nil || u.Scheme != "https" || !adminCenterHosts[u.Host] {
					t.Errorf("AdminCenter = %q, want an https link on one of the admin center hosts", admin)
				}
			}

			for field, link := range allLinks(doc) {
				if strings.Contains(link, "/mem/intune/") {
					t.Errorf("%s = %q uses the retired /mem/intune/ documentation path", field, link)
				}
			}

			seen := map[string]bool{}
			for _, link := range doc.Links.BestPractices {
				if seen[link] {
					t.Errorf("BestPractices repeats %q", link)
				}
				seen[link] = true
			}
		})
	}
}

// TestGraphHandlerSourcesAgreeWithTheirMetadata scans every Graph handler
// source file: the file builds exactly one kind of client (beta xor v1.0), every
// Graph API reference link of the type carries the matching ?view=, and every
// scope named in a "requires '…'" hint is one the type declares. Every
// registered Graph type is matched by exactly one file.
func TestGraphHandlerSourcesAgreeWithTheirMetadata(t *testing.T) {
	registry := NewRegistry(stubCredential{}, "sub-123", false)

	files, err := filepath.Glob(filepath.Join("graph", "*.go"))
	if err != nil {
		t.Fatalf("glob graph handlers: %v", err)
	}

	fileOfType := map[string][]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(raw)
		matches := azureTypeLiteral.FindAllStringSubmatch(src, -1)
		if len(matches) == 0 {
			continue
		}
		for _, m := range matches {
			fileOfType[m[1]] = append(fileOfType[m[1]], file)
		}
		if len(matches) != 1 {
			t.Errorf("%s declares %d azureType literals, want 1", file, len(matches))
			continue
		}
		resourceType := matches[0][1]

		beta := strings.Contains(src, "newBetaGraphClient(") || strings.Contains(src, "msgraph-beta-sdk-go")
		v1 := strings.Contains(src, "newGraphClient(") || strings.Contains(src, "msgraph-sdk-go")
		if beta == v1 {
			t.Errorf("%s (%s) must use the beta client xor the v1.0 client (beta=%v, v1.0=%v)", file, resourceType, beta, v1)
			continue
		}
		wantView := "view=graph-rest-1.0"
		if beta {
			wantView = "view=graph-rest-beta"
		}
		for _, m := range graphAPILink.FindAllStringSubmatch(src, -1) {
			if !strings.HasSuffix(m[1], "?"+wantView) {
				t.Errorf("%s (%s): link %q must end in ?%s", file, resourceType, m[1], wantView)
			}
		}

		handler, err := registry.Get(resourceType)
		if err != nil {
			t.Errorf("%s declares %q, which is not registered: %v", file, resourceType, err)
			continue
		}
		documented, ok := handler.(models.Documented)
		if !ok {
			continue
		}
		declared := map[string]bool{}
		for _, p := range documented.Documentation().RequiredPermissions {
			declared[p] = true
		}
		for _, m := range requiresHint.FindAllStringSubmatch(src, -1) {
			if !declared[m[1]] {
				t.Errorf("%s (%s): hint names %q, which RequiredPermissions %v does not list", file, resourceType, m[1], documented.Documentation().RequiredPermissions)
			}
		}
	}

	for _, resourceType := range registry.GetAllTypes() {
		if models.DetectAPIType(resourceType) != models.APIMicrosoftGraph {
			continue
		}
		if got := len(fileOfType[resourceType]); got != 1 {
			t.Errorf("Graph type %q is matched by %d source files %v, want exactly 1", resourceType, got, fileOfType[resourceType])
		}
	}
}

// allLinks returns every non-empty link of a documentation, keyed by its field.
func allLinks(doc models.ResourceDocumentation) map[string]string {
	links := map[string]string{
		"EndpointDocs":    doc.Links.EndpointDocs,
		"SchemaReference": doc.Links.SchemaReference,
		"Permissions":     doc.Links.Permissions,
		"AdminCenter":     doc.Links.AdminCenter,
	}
	for i, link := range doc.Links.BestPractices {
		links[fmt.Sprintf("BestPractices[%d]", i)] = link
	}
	return links
}
