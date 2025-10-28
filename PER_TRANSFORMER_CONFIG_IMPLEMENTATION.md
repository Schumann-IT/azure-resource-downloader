# Per-Transformer Configuration - Implementation Summary

## Overview

Transformers are now configured individually with their own settings, providing a modular design where each transformer is self-contained.

## Architecture

### Core Types (`internal/models/types.go`)

```go
// TransformerConfig holds configuration for a single transformer
type TransformerConfig struct {
    Name   string                 // e.g., "cleaning", "id-resolution"
    Config map[string]interface{} // Transformer-specific configuration
}

// CleaningConfig - configuration for cleaning transformer
type CleaningConfig struct {
    ExcludeKeys       []string
    ExcludeKeysByType map[string][]string
    CleanEmpty        bool // If true, remove keys with empty values (null, [], "", {})
}

// TerraformImportConfig - configuration for terraform-import transformer
type TerraformImportConfig struct {
    TargetFormat string // e.g., "{resource_type}.{name}"
}
```

### PipelineConfig

```go
type PipelineConfig struct {
    OutputDir          string
    WorkerCount        int
    Timeout            time.Duration
    DryRun             bool
    SubscriptionID     string
    TransformerConfigs []TransformerConfig // List of transformer configurations
}
```

### Helper Functions

```go
// Get default transformers (all with empty config)
func DefaultTransformerConfigs() []TransformerConfig

// Find a transformer config by name
func GetTransformerConfig(configs []TransformerConfig, name TransformerType) *TransformerConfig

// Check if a transformer is configured
func HasTransformer(configs []TransformerConfig, name TransformerType) bool

// Parse cleaning-specific configuration
func ParseCleaningConfig(config map[string]interface{}) *CleaningConfig

// Parse terraform-import-specific configuration
func ParseTerraformImportConfig(config map[string]interface{}) *TerraformImportConfig
```

## Transformer Implementation

### Cleaning Transformer

```go
if cleaningConfig := models.GetTransformerConfig(t.transformerConfigs, models.TransformerCleaning); cleaningConfig != nil {
    config := models.ParseCleaningConfig(cleaningConfig.Config)
    
    // Get type-specific exclusions for this resource
    normalizedType := strings.ToLower(fetchResult.ResourceType)
    typeSpecificKeys := config.ExcludeKeysByType[normalizedType]
    
    // Apply cleaning with configured exclusions and clean-empty flag
    processedData = transform.CleanProperties(processedData, config.ExcludeKeys, typeSpecificKeys, config.CleanEmpty)
}
```

### Terraform Import Transformer

```go
if importConfig := models.GetTransformerConfig(t.transformerConfigs, models.TransformerTerraformImport); importConfig != nil {
    config := models.ParseTerraformImportConfig(importConfig.Config)
    
    // Generate import with configured format
    terraformImport = transform.GenerateTerraformImportBlock(
        terraformResourceType,
        sanitizedName,
        fetchResult.ResourceID,
        config.TargetFormat, // Uses transformer-specific format
    )
}
```

## Configuration Loading (`cmd/download.go`)

```go
func buildTransformerConfigs() []models.TransformerConfig {
    if !viper.IsSet("transformers") {
        return models.DefaultTransformerConfigs()
    }

    transformersConfig := viper.Get("transformers")
    
    switch v := transformersConfig.(type) {
    case []interface{}:
        for _, item := range v {
            if itemMap, ok := item.(map[string]interface{}); ok {
                // Full config with name + settings
                name := itemMap["name"].(string)
                config := extractConfig(itemMap) // Remove "name", keep rest
                
                configs = append(configs, models.TransformerConfig{
                    Name: name,
                    Config: config,
                })
            } else if name, ok := item.(string); ok {
                // Simple string (no config)
                configs = append(configs, models.TransformerConfig{
                    Name: name,
                    Config: map[string]interface{}{},
                })
            }
        }
    }
    
    return configs
}
```

## Configuration Format

### YAML Format

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
        - managedBy
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
    target-format: "{resource_type}.{name}"
```

### Simple Format (No Config)

```yaml
transformers:
  - cleaning
  - id-resolution
  - name-sanitization
  - terraform-import
```

## Benefits

1. **Modular Design** - Each transformer is self-contained
2. **Type Safety** - Config parsing functions provide structure
3. **Extensible** - Easy to add new transformers with their own config
4. **Flexible** - Mix simple and complex configurations
5. **Clear Separation** - No global settings polluting pipeline config

## Comparison with Previous Approach

### Old (Global Config)

```yaml
exclude-keys:
  - etag
exclude-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - id
import-target-format: "{resource_type}.{name}"

transformers:
  - cleaning
  - id-resolution
  - terraform-import
```

**Problems:**
- Global settings affect all transformers
- Unclear which settings belong to which transformer
- Hard to configure transformers differently

### New (Per-Transformer Config)

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
    clean-empty: true
  - name: id-resolution
  - name: terraform-import
    target-format: "{resource_type}.{name}"
```

**Benefits:**
- Each transformer's settings are co-located
- Clear ownership of configuration
- Easy to understand and modify
- Transformers are truly independent

## Files Modified

1. **internal/models/types.go**
   - Removed global exclude keys from PipelineConfig
   - Added TransformerConfig struct
   - Added CleaningConfig and TerraformImportConfig
   - Added helper functions for config parsing

2. **internal/pipeline/transformer.go**
   - Changed from global settings to per-transformer config
   - Parse configuration for each transformer independently
   - Each transformer checks its own config

3. **internal/pipeline/pipeline.go**
   - Pass TransformerConfigs instead of individual settings
   - Apply defaults if no configs specified

4. **cmd/download.go**
   - Added buildTransformerConfigs() function
   - Parse YAML transformer configurations
   - Support both full and simple config formats

5. **cmd/root.go**
   - Removed global --exclude-keys flag
   - Removed --import-target-format flag
   - Removed --transformers flag
   - Configuration is now file-based only

6. **.azure-rd.example.yaml**
   - Comprehensive examples of per-transformer config
   - Shows both simple and complex configurations

## Future Extensibility

Adding a new transformer with custom configuration:

```go
// 1. Define config type
type MyTransformerConfig struct {
    Setting1 string
    Setting2 int
}

// 2. Add parser function
func ParseMyTransformerConfig(config map[string]interface{}) *MyTransformerConfig {
    // Parse config map
}

// 3. Use in transformer
if myConfig := models.GetTransformerConfig(t.transformerConfigs, models.TransformerMyNew); myConfig != nil {
    config := models.ParseMyTransformerConfig(myConfig.Config)
    // Use config.Setting1, config.Setting2
}
```

## Testing

No special testing required - the transformation logic remains the same, only the configuration source changed from global to per-transformer.

## Documentation

- **[docs/TRANSFORMER_CONFIGURATION.md](./docs/TRANSFORMER_CONFIGURATION.md)** - Complete configuration reference
- **[.azure-rd.example.yaml](./.azure-rd.example.yaml)** - Configuration examples
- **[README.md](./README.md)** - Quick start guide

## Summary

The refactoring achieves true modularity where each transformer:
- Has its own configuration schema
- Is independently configurable
- Owns its settings completely
- Can be easily added/removed/modified

This design is more maintainable, clearer, and more extensible than the previous global configuration approach.

