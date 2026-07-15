package graph

import "azure-resource-downloader/internal/models"

// docMeta builds the per-type documentation metadata used to produce a resource
// type's dedicated documentation prompt. Each Graph resource type's constructor
// calls this to set the GraphCollectionHandler.documentation field, so every
// type carries (and can override) its own prompt with a type-specific purpose,
// notable settings, embedded payloads to expand and curated reference links.
// AzureType is filled in from the handler when the prompt is
// built. Pass nil for keySettings/embeddedPayloads and a zero ResourceLinks
// when not relevant.
func docMeta(purpose string, keySettings, embeddedPayloads []string, links models.ResourceLinks) models.ResourceDocumentation {
	return models.ResourceDocumentation{
		Purpose:          purpose,
		KeySettings:      keySettings,
		EmbeddedPayloads: embeddedPayloads,
		Links:            links,
	}
}
