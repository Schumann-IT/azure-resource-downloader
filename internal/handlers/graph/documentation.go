package graph

import "azure-resource-downloader/internal/models"

// docMeta builds the per-type documentation metadata used to produce a resource
// type's dedicated documentation prompt. Each Graph resource type's constructor
// calls this to set the GraphCollectionHandler.documentation field, so every
// type carries (and can override) its own prompt with a type-specific purpose,
// notable settings and embedded payloads to expand. AzureType/TerraformType are
// filled in from the handler when the prompt is built. Pass nil for
// keySettings/embeddedPayloads when not relevant.
func docMeta(purpose string, keySettings, embeddedPayloads []string) models.ResourceDocumentation {
	return models.ResourceDocumentation{
		Purpose:          purpose,
		KeySettings:      keySettings,
		EmbeddedPayloads: embeddedPayloads,
	}
}
