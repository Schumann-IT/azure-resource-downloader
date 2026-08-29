package models

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

// defaultPromptTemplateText is the default documentation prompt template,
// embedded from documentation_prompt.tmpl. It uses Go text/template syntax
// with ResourceDocumentation as its data and a `join` helper (strings.Join).
//
//go:embed documentation_prompt.tmpl
var defaultPromptTemplateText string

// defaultPromptTemplate is the pre-parsed default documentation prompt template.
var defaultPromptTemplate = template.Must(parsePromptTemplate(defaultPromptTemplateText))

// DefaultDocumentationPromptTemplate returns the text of the default
// documentation prompt template. Resource types that want to customize only
// parts of the prompt can start from this text and set the result as their
// ResourceDocumentation.Template override.
func DefaultDocumentationPromptTemplate() string {
	return defaultPromptTemplateText
}

// parsePromptTemplate parses a documentation prompt template with the helper
// functions available to all prompt templates (currently `join`, mapping to
// strings.Join).
func parsePromptTemplate(text string) (*template.Template, error) {
	return template.New("documentation-prompt").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(text)
}

// ResourceLinks holds curated reference URLs for a resource type. All fields
// are optional; empty strings and nil slices are silently omitted from the
// generated prompt.
type ResourceLinks struct {
	// EndpointDocs is the canonical Microsoft Learn / Graph REST API reference
	// URL for this resource type.
	EndpointDocs string
	// BestPractices are links to hardening guides, security baselines, CIS
	// benchmarks, or any other best-practice documentation relevant to this
	// resource type.
	BestPractices []string
	// SchemaReference is the OData $metadata endpoint, ARM type schema, or
	// Graph schema explorer URL for this resource type. Helps the LLM resolve
	// unfamiliar properties to their canonical type definitions.
	SchemaReference string
	// Permissions is the Microsoft Learn page that lists the API permissions
	// and RBAC roles required to read/write this resource type.
	Permissions string
}

// ResourceDocumentation holds the per-resource-type metadata used to build a
// dedicated documentation LLM prompt. AzureType identifies the resource type;
// all other fields tailor the prompt so each resource type gets its own prompt
// rather than a single generic one. Every field except AzureType and Purpose
// is optional and omitted from the prompt when empty.
type ResourceDocumentation struct {
	// AzureType is the Azure/Microsoft Graph resource type (e.g. "Microsoft.Graph/deviceConfigurations").
	AzureType string
	// Purpose is a short, type-specific description of what the resource is and what it controls.
	Purpose string
	// KeySettings lists the settings most important for this type, to be given particular attention.
	KeySettings []string
	// EmbeddedPayloads lists the encoded/embedded properties this type carries that must be decoded and expanded
	// (e.g. "omaSettings", "configurationXml", base64 "payload").
	EmbeddedPayloads []string
	// RequiredPermissions lists the permission scopes or RBAC roles needed to
	// read this resource type (e.g. "DeviceManagementConfiguration.Read.All").
	RequiredPermissions []string
	// Lifecycle describes type-specific lifecycle and operational facts the
	// model cannot infer from the YAML: deprecation/migration status, effect of
	// deleting or unassigning the resource, renewal/expiry obligations, etc.
	Lifecycle []string
	// RelatedTypes lists other exported resource types this type references or
	// is referenced by (e.g. assignment target groups), so documentation can
	// cross-reference sibling exports instead of guessing.
	RelatedTypes []string
	// SubtypeNote explains how to handle polymorphic types whose concrete
	// schema is selected by @odata.type (e.g. mobileApps, deviceConfigurations).
	SubtypeNote string
	// Links holds curated reference URLs for this resource type. All subfields
	// are optional and are omitted from the prompt when empty.
	Links ResourceLinks
	// Template optionally overrides the default documentation prompt template
	// for this resource type. It uses Go text/template syntax, is executed with
	// this ResourceDocumentation as data and has a `join` helper (strings.Join)
	// available. When empty, the embedded default template is used; see
	// DefaultDocumentationPromptTemplate for its text.
	Template string
}

// BuildDocumentationPrompt returns a dedicated LLM prompt for the given resource
// type by executing a Go text/template with the ResourceDocumentation as data.
// By default the embedded template (documentation_prompt.tmpl) is used; a
// resource type can supply its own template via the Template field. The prompt
// is tailored using the type's Purpose, KeySettings, EmbeddedPayloads,
// RequiredPermissions, Lifecycle, RelatedTypes, SubtypeNote and Links so every
// resource type produces its own prompt.
//
// Templates are developer-authored constants, so an invalid Template override
// or a failing execution panics (fail-fast, analogous to template.Must); this
// surfaces immediately in tests rather than silently producing broken prompts.
func BuildDocumentationPrompt(doc ResourceDocumentation) string {
	tmpl := defaultPromptTemplate
	if doc.Template != "" {
		override, err := parsePromptTemplate(doc.Template)
		if err != nil {
			panic(fmt.Sprintf("invalid documentation prompt template for %s: %v", doc.AzureType, err))
		}
		tmpl = override
	}

	var b strings.Builder
	if err := tmpl.Execute(&b, doc); err != nil {
		panic(fmt.Sprintf("failed to execute documentation prompt template for %s: %v", doc.AzureType, err))
	}
	return strings.TrimSuffix(b.String(), "\n")
}
