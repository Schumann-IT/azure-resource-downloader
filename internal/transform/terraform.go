package transform

import (
	"fmt"
	"strings"

	"azure-resource-downloader/internal/logger"
)

// GenerateTerraformImport creates a Terraform import statement
func GenerateTerraformImport(terraformResourceType, resourceName, azureResourceID string) string {
	sanitizedName := SanitizeTerraformName(resourceName)

	return fmt.Sprintf("terraform import %s.%s \"%s\"",
		terraformResourceType,
		sanitizedName,
		azureResourceID,
	)
}

// GenerateTerraformImportBlock creates a Terraform import block (Terraform 1.5+)
func GenerateTerraformImportBlock(terraformResourceType, resourceName, azureResourceID, targetFormat string) string {
	log := logger.Default

	sanitizedName := SanitizeTerraformName(resourceName)

	// Default format if not specified
	if targetFormat == "" {
		targetFormat = "{resource_type}.{name}"
	}

	// Replace template variables
	targetAddress := strings.ReplaceAll(targetFormat, "{resource_type}", terraformResourceType)
	targetAddress = strings.ReplaceAll(targetAddress, "{name}", sanitizedName)

	log.Debug("Generated Terraform import block",
		"resource_type", terraformResourceType,
		"resource_name", resourceName,
		"sanitized_name", sanitizedName,
		"target_address", targetAddress,
		"target_format", targetFormat)

	var sb strings.Builder

	sb.WriteString("import {\n")
	fmt.Fprintf(&sb, "  to = %s\n", targetAddress)
	fmt.Fprintf(&sb, "  id = %q\n", azureResourceID)
	sb.WriteString("}\n")

	return sb.String()
}

// GenerateTerraformResourceStub creates a basic Terraform resource stub
func GenerateTerraformResourceStub(terraformResourceType, resourceName string, properties map[string]interface{}) string {
	sanitizedName := SanitizeTerraformName(resourceName)

	var sb strings.Builder

	fmt.Fprintf(&sb, "resource %q %q {\n", terraformResourceType, sanitizedName)

	// Add common properties if available
	if name, ok := properties["name"].(string); ok {
		fmt.Fprintf(&sb, "  name = %q\n", name)
	}

	if location, ok := properties["location"].(string); ok {
		fmt.Fprintf(&sb, "  location = %q\n", location)
	}

	if rgName := extractResourceGroup(properties); rgName != "" {
		fmt.Fprintf(&sb, "  resource_group_name = %q\n", rgName)
	}

	sb.WriteString("\n  # Additional properties imported from Azure\n")
	sb.WriteString("  # Run 'terraform plan' after import to see all attributes\n")
	sb.WriteString("}\n")

	return sb.String()
}

// extractResourceGroup attempts to extract resource group name from properties
func extractResourceGroup(properties map[string]interface{}) string {
	// Try common property paths
	if id, ok := properties["id"].(string); ok {
		parts := strings.Split(id, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	if rg, ok := properties["resourceGroup"].(string); ok {
		return rg
	}

	return ""
}
