package transform

import (
	"fmt"
	"strings"
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
	sanitizedName := SanitizeTerraformName(resourceName)

	// Default format if not specified
	if targetFormat == "" {
		targetFormat = "{resource_type}.{name}"
	}

	// Replace template variables
	targetAddress := strings.ReplaceAll(targetFormat, "{resource_type}", terraformResourceType)
	targetAddress = strings.ReplaceAll(targetAddress, "{name}", sanitizedName)

	var sb strings.Builder

	sb.WriteString("import {\n")
	sb.WriteString(fmt.Sprintf("  to = %s\n", targetAddress))
	sb.WriteString(fmt.Sprintf("  id = \"%s\"\n", azureResourceID))
	sb.WriteString("}\n")

	return sb.String()
}

// GenerateTerraformResourceStub creates a basic Terraform resource stub
func GenerateTerraformResourceStub(terraformResourceType, resourceName string, properties map[string]interface{}) string {
	sanitizedName := SanitizeTerraformName(resourceName)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("resource \"%s\" \"%s\" {\n", terraformResourceType, sanitizedName))

	// Add common properties if available
	if name, ok := properties["name"].(string); ok {
		sb.WriteString(fmt.Sprintf("  name = \"%s\"\n", name))
	}

	if location, ok := properties["location"].(string); ok {
		sb.WriteString(fmt.Sprintf("  location = \"%s\"\n", location))
	}

	if rgName := extractResourceGroup(properties); rgName != "" {
		sb.WriteString(fmt.Sprintf("  resource_group_name = \"%s\"\n", rgName))
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
