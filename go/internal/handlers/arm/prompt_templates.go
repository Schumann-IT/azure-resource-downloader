package arm

import _ "embed"

// armPromptTemplateText is the shared documentation prompt template for all
// ARM resource types. The default template (internal/models/documentation_prompt.tmpl)
// assumes Intune/Entra-style assignments and settings payloads, which do not
// exist for ARM resources; this template frames the documentation around ARM
// concepts (RBAC, locks, tags, SKU, network/encryption posture) instead.
// Handlers wire it up via the Template field of models.ResourceDocumentation.
// Like every prompt template it is parsed with the shared partials of
// internal/models/prompt_partials.tmpl and calls them for the header, the
// reference links, the shared rules and the closed-set paragraph.
//
//go:embed arm_prompt.tmpl
var armPromptTemplateText string
