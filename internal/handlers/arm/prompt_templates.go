package arm

import _ "embed"

// armPromptTemplateText is the shared documentation prompt template for all
// ARM resource types. The default template (internal/models/documentation_prompt.tmpl)
// assumes Intune/Entra-style assignments and settings payloads, which do not
// exist for ARM resources; this template frames the documentation around ARM
// concepts (RBAC, locks, tags, SKU, network/encryption posture) instead.
// Handlers wire it up via the Template field of models.ResourceDocumentation.
//
//go:embed arm_prompt.tmpl
var armPromptTemplateText string
