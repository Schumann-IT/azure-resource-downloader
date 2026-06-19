package models

import (
	"fmt"
	"strings"
)

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
	// TerraformDocs is the Terraform Registry page for the corresponding
	// azurerm or microsoft365 provider resource.
	TerraformDocs string
	// SchemaReference is the OData $metadata endpoint, ARM type schema, or
	// Graph schema explorer URL for this resource type. Helps the LLM resolve
	// unfamiliar properties to their canonical type definitions.
	SchemaReference string
	// Permissions is the Microsoft Learn page that lists the API permissions
	// and RBAC roles required to read/write this resource type.
	Permissions string
}

// ResourceDocumentation holds the per-resource-type metadata used to build a
// dedicated documentation LLM prompt. AzureType and TerraformType identify the
// resource type; Purpose, KeySettings, EmbeddedPayloads and Links tailor the
// prompt so each resource type gets its own prompt rather than a single generic
// one.
type ResourceDocumentation struct {
	// AzureType is the Azure/Microsoft Graph resource type (e.g. "Microsoft.Graph/deviceConfigurations").
	AzureType string
	// TerraformType is the Terraform resource type; empty when the type has no Terraform representation.
	TerraformType string
	// Purpose is a short, type-specific description of what the resource is and what it controls.
	Purpose string
	// KeySettings lists the settings most important for this type, to be given particular attention.
	KeySettings []string
	// EmbeddedPayloads lists the encoded/embedded properties this type carries that must be decoded and expanded
	// (e.g. "omaSettings", "configurationXml", base64 "payload").
	EmbeddedPayloads []string
	// Links holds curated reference URLs for this resource type. All subfields
	// are optional and are omitted from the prompt when empty.
	Links ResourceLinks
}

// BuildDocumentationPrompt returns a dedicated LLM prompt for the given resource
// type. The prompt is tailored using the type's Purpose, KeySettings,
// EmbeddedPayloads and Links so every resource type produces its own prompt. It
// asks the model to document every setting with best-practice guidance,
// Microsoft documentation links, and fully expanded embedded payloads.
//
// TerraformType may be empty for resource types with no Terraform
// representation; the corresponding line is then omitted. All fields of Links
// are optional and are omitted from the prompt when empty.
func BuildDocumentationPrompt(doc ResourceDocumentation) string {
	var b strings.Builder

	b.WriteString("You are a senior Microsoft cloud and endpoint-management consultant. ")
	b.WriteString("Generate clear, accurate end-user documentation for the attached resource configuration.\n\n")

	fmt.Fprintf(&b, "Azure resource type: %s\n", doc.AzureType)
	if doc.TerraformType != "" {
		fmt.Fprintf(&b, "Terraform resource type: %s\n", doc.TerraformType)
	}
	if doc.Purpose != "" {
		fmt.Fprintf(&b, "About this resource type: %s\n", doc.Purpose)
	}

	if l := doc.Links; l.EndpointDocs != "" || l.TerraformDocs != "" || l.SchemaReference != "" || l.Permissions != "" || len(l.BestPractices) > 0 {
		b.WriteString("\nReference material for this resource type (treat these as authoritative; prefer them over recalled knowledge):\n")
		if l.EndpointDocs != "" {
			fmt.Fprintf(&b, "- API reference: %s\n", l.EndpointDocs)
		}
		if l.TerraformDocs != "" {
			fmt.Fprintf(&b, "- Terraform registry: %s\n", l.TerraformDocs)
		}
		if l.SchemaReference != "" {
			fmt.Fprintf(&b, "- Schema reference: %s\n", l.SchemaReference)
		}
		if l.Permissions != "" {
			fmt.Fprintf(&b, "- Required permissions: %s\n", l.Permissions)
		}
		for _, bp := range l.BestPractices {
			fmt.Fprintf(&b, "- Best-practice baseline: %s\n", bp)
		}
	}

	b.WriteString("\n")

	b.WriteString("The configuration is provided as a YAML file exported by azure-resource-downloader. ")
	b.WriteString("Produce well-structured Markdown documentation that:\n\n")

	b.WriteString("1. Opens with a short summary of what this specific resource is and its purpose within a tenant.\n")
	b.WriteString("2. Documents EVERY setting/property present in the YAML in a table with the columns: ")
	b.WriteString("Setting (YAML path), Configured value, What it does, Recommended/best-practice value, Reference. ")
	b.WriteString("Do not omit any property; if a property is unfamiliar, infer its meaning from the Microsoft Graph/ARM schema and say so explicitly.\n")
	b.WriteString("3. Links each setting to the authoritative Microsoft documentation (Microsoft Learn) and, where relevant, to a recognized hardening/best-practice baseline ")
	b.WriteString("(e.g. Microsoft security baselines, CIS Benchmarks). Use real, verifiable URLs; if you are unsure of an exact URL, link to the closest canonical Microsoft Learn page and flag it as approximate.\n")

	b.WriteString("4. Fully expands and explains any embedded or encoded payloads")
	if len(doc.EmbeddedPayloads) > 0 {
		fmt.Fprintf(&b, " — for this resource type pay particular attention to: %s", strings.Join(doc.EmbeddedPayloads, ", "))
	} else {
		b.WriteString(" — for example `configurationXml`, `omaSettings`, `payloadJson`, custom OMA-URI values and base64/`payload` blobs")
	}
	b.WriteString(". Decode and pretty-print them, then document each contained key/value the same way as the top-level settings.\n")

	b.WriteString("5. Calls out security-sensitive settings (secrets, certificates, encryption, conditional-access conditions, etc.) and any deviations from recommended baselines, including the security impact.")
	if len(doc.KeySettings) > 0 {
		fmt.Fprintf(&b, " For this resource type, give particular attention to: %s.", strings.Join(doc.KeySettings, ", "))
	}
	b.WriteString("\n")
	b.WriteString("6. Notes any assignments/targeting present and explains what they mean.\n\n")

	b.WriteString("Only describe settings that are actually present; never invent values. Where a value is masked or redacted, state that explicitly.")

	return b.String()
}
