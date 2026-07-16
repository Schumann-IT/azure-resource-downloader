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
}

// BuildDocumentationPrompt returns a dedicated LLM prompt for the given resource
// type. The prompt is tailored using the type's Purpose, KeySettings,
// EmbeddedPayloads, RequiredPermissions, Lifecycle, RelatedTypes, SubtypeNote
// and Links so every resource type produces its own prompt. It asks the model
// to document every setting with best-practice guidance, Microsoft
// documentation links, fully expanded embedded payloads and lifecycle notes.
//
// All fields except AzureType and Purpose are optional and are omitted from
// the prompt when empty.
func BuildDocumentationPrompt(doc ResourceDocumentation) string {
	var b strings.Builder

	b.WriteString("You are a senior Microsoft cloud and endpoint-management consultant. ")
	b.WriteString("Generate clear, accurate end-user documentation for the attached resource configuration.\n\n")

	fmt.Fprintf(&b, "Azure resource type: %s\n", doc.AzureType)
	if doc.Purpose != "" {
		fmt.Fprintf(&b, "About this resource type: %s\n", doc.Purpose)
	}
	if doc.SubtypeNote != "" {
		fmt.Fprintf(&b, "Subtype guidance: %s\n", doc.SubtypeNote)
	}
	if len(doc.RequiredPermissions) > 0 {
		b.WriteString("Permissions required to read this resource type:\n")
		for _, p := range doc.RequiredPermissions {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	if len(doc.Lifecycle) > 0 {
		b.WriteString("Lifecycle notes for this resource type:\n")
		for _, p := range doc.Lifecycle {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}

	if l := doc.Links; l.EndpointDocs != "" || l.SchemaReference != "" || l.Permissions != "" || len(l.BestPractices) > 0 {
		b.WriteString("\nReference material for this resource type (treat these as authoritative; prefer them over recalled knowledge):\n")
		if l.EndpointDocs != "" {
			fmt.Fprintf(&b, "- API reference: %s\n", l.EndpointDocs)
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

	if len(doc.RelatedTypes) > 0 {
		b.WriteString("\nRelated resource types exported alongside this one (cross-reference their YAML directories instead of guessing):\n")
		for _, rt := range doc.RelatedTypes {
			fmt.Fprintf(&b, "- %s\n", rt)
		}
	}

	b.WriteString("\n")

	b.WriteString("The configuration is provided as a YAML file exported by azure-resource-downloader. ")
	b.WriteString("Produce well-structured Markdown documentation with this layout:\n\n")

	b.WriteString("- An H1 title set to the resource's display name.\n")
	b.WriteString("- Directly below the title, without a heading: a short summary paragraph describing what this specific resource is and its purpose within the tenant.\n")
	b.WriteString("- Directly after the summary paragraph, a metadata table stating the resource type, the concrete subtype (`@odata.type`, if present), the resource ID, ")
	b.WriteString("and other identifying top-level fields where present (e.g. platform, technologies).\n")
	b.WriteString("- Directly after the metadata table, without a heading: a short explanation of assignments/targeting and what they mean, followed by a table of any assignments/targeting present, ")
	b.WriteString("(include/exclude targets, assignment filters). Assignment targets contain group IDs only — group names are NOT resolved in the export ")
	b.WriteString("(groups are exported separately as Microsoft.Graph/groups); never invent group names.\n\n")

	b.WriteString("Then the following H2 sections, unnumbered, in this order:\n\n")

	b.WriteString("References:\n")
	b.WriteString("- link each setting to the authoritative Microsoft documentation (Microsoft Learn)\n")
	b.WriteString("- and, where relevant, to a recognized hardening/best-practice baseline (e.g. Microsoft security baselines, CIS Benchmarks)\n")
	b.WriteString("- Use real, verifiable URLs; if you are unsure of an exact URL, link to the closest canonical Microsoft Learn page and flag it as approximate.\n")
	b.WriteString("\n")
	b.WriteString("Lifecycle & operations:\n")
	b.WriteString("- document operational guidance: deprecation or migration status, what happens when the resource is deleted or unassigned\n")
	b.WriteString("- renewal/expiry obligations\n")
	b.WriteString("- recommended review cadence.\n")
	b.WriteString("\n")
	b.WriteString("Security:\n")
	b.WriteString("- call out security-sensitive settings (secrets, certificates, encryption, conditional-access conditions, etc.)\n")
	b.WriteString("- any deviations from recommended baselines, including the security impact.\n")
	b.WriteString("\n")
	b.WriteString("Settings:\n")
	b.WriteString("- document EVERY setting/property present in the YAML.\n")
	if len(doc.KeySettings) > 0 {
		fmt.Fprintf(&b, "- give particular attention to: %s.\n", strings.Join(doc.KeySettings, ", "))
	}
	b.WriteString("- Render each setting as a collapsible HTML `<details>` block, collapsed by default, so the reader can click a setting to unfold it: ")
	b.WriteString("the `<summary>` holds the setting key (YAML path) and its configured value; ")
	b.WriteString("the expanded body documents what the setting does, the recommended/best-practice value and a reference link.\n")
	b.WriteString("- If the YAML carries an `@odata.type`, first identify the concrete subtype and document against that subtype's schema.\n")
	b.WriteString("- Do not omit any property; if a property is unfamiliar, infer its meaning from the Microsoft Graph/ARM schema and say so explicitly.\n")
	if len(doc.EmbeddedPayloads) > 0 {
		fmt.Fprintf(&b, "- This resource carries embedded or encoded payloads: %s —", strings.Join(doc.EmbeddedPayloads, ", "))
		b.WriteString(" decode and pretty-print it inside that setting's expanded body and document each contained key/value the same way, ")
		b.WriteString("using nested `<details>` blocks for the payload's keys where that aids readability.\n")
	}
	b.WriteString("- If the YAML references an externally decoded sidecar file (e.g. a `.ps1`/`.sh`/`.mobileconfig` written next to the YAML), document its contents inside the owning setting's expanded body.\n")
	b.WriteString("- Only describe settings that are actually present; never invent values.\n")
	b.WriteString("- Where a value is masked or redacted by the service, state that explicitly and do not flag it as a misconfiguration.")

	return b.String()
}
