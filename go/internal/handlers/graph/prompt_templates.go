package graph

import _ "embed"

// The default documentation prompt template (internal/models/documentation_prompt.tmpl)
// assumes a policy-like resource with a settings payload and Intune-style
// assignments. The shared templates below override it for resource-type
// families where that framing does not fit; handlers wire them up via the
// Template field of models.ResourceDocumentation.

// singletonPromptTemplateText is the shared prompt template for tenant-wide
// singleton configurations (e.g. organization, authorizationPolicy): one
// instance per tenant, no display name, no assignments.
//
//go:embed singleton_prompt.tmpl
var singletonPromptTemplateText string

// credentialPromptTemplateText is the shared prompt template for service
// credential/token records (e.g. APNs certificate, VPP tokens, DEP tokens):
// documentation focuses on validity, renewal and expiry impact.
//
//go:embed credential_prompt.tmpl
var credentialPromptTemplateText string

// recordPromptTemplateText is the shared prompt template for inventory and
// registry records (e.g. Autopilot device identities, device categories,
// connectors): registered entities without settings payloads or assignments.
//
//go:embed record_prompt.tmpl
var recordPromptTemplateText string

// referencedPromptTemplateText is the shared prompt template for supporting
// objects referenced by ID from other policies (e.g. assignment filters,
// named locations, authentication strengths): they have no assignments of
// their own — other resources point at them.
//
//go:embed referenced_prompt.tmpl
var referencedPromptTemplateText string
