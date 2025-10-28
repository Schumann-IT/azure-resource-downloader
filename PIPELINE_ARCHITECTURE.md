# Pipeline Architecture

## Overview

The Azure Resource Downloader uses a **streaming pipeline architecture** with three concurrent stages connected via Go channels. This enables maximum parallelism and efficient resource processing.

```
┌─────────────────────────────────────────────────────────────────┐
│                      PIPELINE ARCHITECTURE                       │
└─────────────────────────────────────────────────────────────────┘

Input: [FetchRequest] → Channel → [Fetcher] → Channel → [Transformer] → Channel → [Writer] → Output: [WriteResult]

All stages run CONCURRENTLY via Go channels for streaming data flow.
```

---

## Pipeline Flow

### Main Execution (`pipeline.go`)

```go
// Stage 1: Fetch (starts immediately, returns channel)
fetchResults := p.fetcher.Fetch(ctx, requests)

// Stage 2: Transform (starts consuming immediately)
transformResults := p.transformer.Transform(ctx, fetchResults)

// Stage 3: Write (starts consuming immediately)
writeResults := p.writer.Write(ctx, transformResults)
```

**Key Characteristics:**
- ✅ All stages start immediately
- ✅ Stages run concurrently (not sequentially)
- ✅ Resources flow through the pipeline in parallel
- ✅ Each resource: Fetch → Transform → Write
- ✅ Worker pools in each stage for parallel processing

---

## Stage 1: FETCHER (`fetcher.go`)

### Purpose
Retrieve raw resource data from Azure APIs with retry logic and error handling.

### Steps

#### 1.1 **Initialize Worker Pool**
```go
- Create input channel with all fetch requests
- Spawn N worker goroutines (configurable via --workers)
- Each worker independently processes requests
```

#### 1.2 **Parse Resource ID** (per request)
```go
- Extract resource type from Azure resource ID
- Parse components: subscription, resource group, provider, resource name
- Example: "/subscriptions/XXX/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/myaccount"
  → Type: "Microsoft.Storage/storageAccounts"
  → Name: "myaccount"
```

#### 1.3 **Get Handler**
```go
- Look up appropriate handler from registry based on resource type
- Handlers: ResourceGroupHandler, StorageAccountHandler, VirtualMachineHandler, etc.
- Return error if no handler exists for the resource type
```

#### 1.4 **Fetch Resource with Retry**
```go
- Call handler.Fetch(ctx, resourceID) to retrieve raw Azure data
- Retry configuration:
  * Max attempts: 5 (default)
  * Exponential backoff: 1s, 2s, 4s, 8s, 16s
  * Handles rate limits (429), transient errors (500, 502, 503, 504)
  * Context cancellation support
- Log warnings on retries, info on success after retries
```

#### 1.5 **Return Result**
```go
FetchResult {
    ResourceID:   string              // Azure resource ID
    ResourceType: string              // e.g., "Microsoft.Storage/storageAccounts"
    RawData:      interface{}         // Raw Azure SDK response object
    Error:        error               // nil on success
}
```

### Error Handling
- Parse errors → FetchResult with error
- Missing handler → FetchResult with error
- Fetch failures → FetchResult with error (after retries)
- Context cancellation → FetchResult with ctx.Err()

---

## Stage 2: TRANSFORMER (`transformer.go`)

### Purpose
Transform raw Azure data into clean, sanitized format and generate Terraform import statements.

### Steps

#### 2.1 **Initialize Worker Pool**
```go
- Spawn N worker goroutines
- Each worker reads from fetchResults channel
- Process transformations in parallel
```

#### 2.2 **Validate Input** (per fetch result)
```go
- Check if fetch had an error
- If error exists, pass through as TransformResult with error
- Skip transformation for failed fetches
```

#### 2.3 **Get Handler**
```go
- Look up handler for the resource type
- Same handler used in fetch stage
```

#### 2.4 **Transform Raw Data**
```go
- Call handler.Transform(rawData)
- Handler converts SDK object → clean map[string]interface{}
- Extracts: ID, Name, DisplayName, Properties
- Handler-specific logic per resource type
```

#### 2.5 **Apply Type-Specific Exclusions**
```go
- Get global exclude keys (from config/CLI)
- Get type-specific exclude keys (from exclude-keys-by-type config)
- Example type-specific:
  * Microsoft.Resources/resourceGroups: [id, managedBy]
  * Microsoft.Storage/storageAccounts: [primaryEndpoints, secondaryEndpoints]
- Merge global + type-specific exclusions
```

#### 2.6 **Clean Properties**
```go
transform.CleanProperties(properties, globalKeys, typeSpecificKeys)
- Remove excluded keys recursively
- Remove null/empty values
- Remove Azure metadata (provisioningState, etag, systemData)
- Simplify nested structures
```

#### 2.7 **Resolve Azure IDs**
```go
azure.ResolveIDsInProperties(cleanedData)
- Find Azure resource IDs in properties
- Convert IDs to friendly names
- Example: "/subscriptions/XXX/resourceGroups/my-rg" → "my-rg"
- Helps with Terraform readability
```

#### 2.8 **Sanitize Names**
```go
transform.SanitizeFileName(displayName)
- Convert to valid file/Terraform names
- Replace special characters: spaces → underscores, remove @!#$%
- Lowercase: "My-Resource-Group" → "my_resource_group"
- Remove leading/trailing underscores
```

#### 2.9 **Generate Terraform Import Block**
```go
transform.GenerateTerraformImportBlock(
    terraformResourceType,   // e.g., "azurerm_resource_group"
    sanitizedName,           // e.g., "my_rg"
    resourceID,              // Azure resource ID
    importTargetFormat       // Template: "{resource_type}.{name}" or custom
)

Output example (default format):
import {
  to = azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}

Output example (module format):
import {
  to = module["my_rg"].azurerm_resource_group.this
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

#### 2.10 **Return Result**
```go
TransformResult {
    ResourceID:            string                 // Azure resource ID
    ResourceType:          string                 // Azure type
    DisplayName:           string                 // Human-readable name
    SanitizedName:         string                 // File/Terraform-safe name
    CleanedData:           map[string]interface{} // Cleaned properties
    TerraformImport:       string                 // Import block
    TerraformResourceType: string                 // TF resource type
    Error:                 error                  // nil on success
}
```

### Error Handling
- Input error → Pass through with error
- Missing handler → TransformResult with error
- Transform failure → TransformResult with error
- Context cancellation → TransformResult with ctx.Err()

---

## Stage 3: WRITER (`writer.go`)

### Purpose
Write transformed data to disk as YAML files and consolidated Terraform import files.

### Steps

#### 3.1 **Initialize Worker Pool**
```go
- Spawn N worker goroutines
- Create importsByType map for collecting import statements
- Each worker reads from transformResults channel
```

#### 3.2 **Validate Input** (per transform result)
```go
- Check if transform had an error
- If error exists, return WriteResult with error
- Skip writing for failed transforms
```

#### 3.3 **Create Output Directory**
```go
- Build path: outputDir/ResourceType/
- Example: "./output/Microsoft.Resources/resourceGroups/"
- Create directory with os.MkdirAll (0755 permissions)
- Skip if dry-run mode enabled
```

#### 3.4 **Write YAML File**
```go
- Marshal CleanedData to YAML format
- File path: resourceTypeDir/sanitizedName.yaml
- Example: "./output/Microsoft.Resources/resourceGroups/my_rg.yaml"
- Write with 0644 permissions

YAML Content Example:
---
id: /subscriptions/.../resourceGroups/my-rg
name: my-rg
location: eastus
tags:
  environment: production
```

#### 3.5 **Collect Import Statement**
```go
- Thread-safe collection using mutex
- Group by resource type
- Store: DisplayName + TerraformImport block
- Accumulated in memory during worker processing
```

#### 3.6 **Return Individual Result**
```go
WriteResult {
    ResourceID:    string  // Azure resource ID
    YAMLPath:      string  // Path to YAML file
    TerraformPath: string  // Path to import.tf (consolidated)
    Error:         error   // nil on success
}
```

#### 3.7 **Wait for All Workers** (after worker pool completes)
```go
- sync.WaitGroup ensures all workers finish
- All resources have been processed
- All import statements collected
```

#### 3.8 **Write Consolidated Import Files**
```go
writeImportFiles()
- Executes AFTER all workers complete
- One import.tf file per resource type
- Iterates through importsByType map

For each resource type:
  1. Build file path: resourceTypeDir/import.tf
  2. Concatenate all import blocks with comments
  3. Write consolidated file
  4. Log import count

Example output/Microsoft.Resources/resourceGroups/import.tf:
# Terraform import statements
# Generated by azure-resource-downloader

# Import for my-rg
import {
  to = azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}

# Import for another-rg
import {
  to = azurerm_resource_group.another_rg
  id = "/subscriptions/.../resourceGroups/another-rg"
}
```

### Error Handling
- Input error → Pass through with error
- Directory creation failure → WriteResult with error
- YAML marshal failure → WriteResult with error
- File write failure → WriteResult with error
- Import file write failure → Logged but doesn't fail individual resources
- Context cancellation → WriteResult with ctx.Err()

---

## Concurrency Model

### Worker Pools

Each stage uses a configurable number of workers:

```
┌─────────────────────────────────────────────────┐
│              FETCHER (N workers)                │
│  Worker 1  │  Worker 2  │  ...  │  Worker N    │
│      ↓     │      ↓     │   ↓   │      ↓       │
└─────────────────────────────────────────────────┘
                         ↓
           [Channel: FetchResult]
                         ↓
┌─────────────────────────────────────────────────┐
│            TRANSFORMER (N workers)              │
│  Worker 1  │  Worker 2  │  ...  │  Worker N    │
│      ↓     │      ↓     │   ↓   │      ↓       │
└─────────────────────────────────────────────────┘
                         ↓
          [Channel: TransformResult]
                         ↓
┌─────────────────────────────────────────────────┐
│              WRITER (N workers)                 │
│  Worker 1  │  Worker 2  │  ...  │  Worker N    │
│      ↓     │      ↓     │   ↓   │      ↓       │
└─────────────────────────────────────────────────┘
                         ↓
           After all workers complete:
           Write consolidated import.tf files
                         ↓
            [Channel: WriteResult]
```

### Channel-Based Communication

- **Go Channels** connect stages
- **Non-blocking** streaming architecture
- **Backpressure** automatically handled by Go runtime
- **Early results** available immediately (don't wait for all fetches)

### Worker Count Configuration

```bash
# Global worker count
--workers 10

# API-specific workers (config file)
workers-by-api:
  microsoft-graph: 5         # Strict rate limits
  azure-resource-manager: 20 # Generous limits

# Automatic API detection
# Tool detects API type and applies appropriate worker count
```

---

## Performance Optimizations

### 1. **Streaming Pipeline**
- Resources flow through as soon as fetched
- Don't wait for all fetches before transforming
- First resource can be written while last is being fetched

### 2. **Parallel Processing**
- Multiple resources fetched simultaneously
- Multiple resources transformed simultaneously
- Multiple resources written simultaneously

### 3. **Retry Logic with Exponential Backoff**
- Automatic retry on transient failures
- Exponential backoff prevents rate limit cascades
- Respects Azure API rate limits

### 4. **Efficient Error Propagation**
- Errors don't block other resources
- Failed resources return error results
- Pipeline continues processing remaining resources

### 5. **Context-Aware Cancellation**
- Timeout support via context
- Graceful shutdown on Ctrl+C
- Workers respect context cancellation

---

## Output Structure

```
output/
├── Microsoft.Resources/
│   └── resourceGroups/
│       ├── my_rg.yaml                    # Individual resource YAML
│       ├── another_rg.yaml               # Individual resource YAML
│       └── import.tf                     # Consolidated import file
├── Microsoft.Storage/
│   └── storageAccounts/
│       ├── mystorageaccount.yaml         # Individual resource YAML
│       ├── anotherstorageaccount.yaml    # Individual resource YAML
│       └── import.tf                     # Consolidated import file
└── Microsoft.Compute/
    └── virtualMachines/
        ├── my_vm.yaml                    # Individual resource YAML
        └── import.tf                     # Consolidated import file
```

**Key Points:**
- ✅ One YAML file per resource
- ✅ One import.tf per resource type (contains all imports for that type)
- ✅ Directory structure mirrors Azure resource hierarchy
- ✅ Sanitized names for file safety

---

## Error Handling Strategy

### Per-Resource Errors
- Individual resource failures don't stop the pipeline
- Errors captured in result objects
- Pipeline continues processing other resources

### Stage-Level Errors
- Critical errors (context cancellation) stop the stage
- Workers return early on context cancellation
- Downstream stages receive error results

### Summary Reporting
```go
ExecutionSummary {
    TotalResources:      int      // Total resources requested
    SuccessfulResources: int      // Successfully processed
    FailedResources:     int      // Failed at any stage
    Results:             []Result // All individual results
    Errors:              []string // Error messages
}
```

---

## Metrics & Monitoring

### Progress Tracking
- Real-time progress updates every 10%
- Shows: completed/total, percentage, success/failed counts
- Elapsed time tracking

### Performance Metrics
```
Pipeline Metrics:
- Workers: N
- Total resources: X
- Resources/second: Y
- Success rate: Z%
- Average latency: Tms per resource
```

### Logging Levels
- **debug**: Detailed per-resource operations
- **info**: Progress, metrics, pipeline flow
- **warn**: Retries, non-critical issues
- **error**: Failures, critical issues

---

## Configuration Options

### Worker Configuration
```yaml
# Global workers
workers: 10

# API-specific workers
workers-by-api:
  microsoft-graph: 5
  azure-resource-manager: 20
```

### Import Target Format
```yaml
# Default: {resource_type}.{name}
import-target-format: "{resource_type}.{name}"

# Module format
import-target-format: "module[\"{name}\"].{resource_type}.this"
```

### Exclusion Rules
```yaml
# Global exclusions (all resource types)
exclude-keys:
  - provisioningState
  - etag

# Type-specific exclusions
exclude-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - managedBy
  Microsoft.Storage/storageAccounts:
    - primaryEndpoints
```

### Dry Run
```bash
# Preview without writing files
--dry-run
```

### Timeout
```bash
# Set timeout for entire pipeline
--timeout 600  # 600 seconds
```

---

## Summary

The pipeline architecture provides:
- ✅ **High performance** via concurrent processing
- ✅ **Resilience** via retry logic and error isolation
- ✅ **Flexibility** via configurable workers and formats
- ✅ **Observability** via comprehensive logging and metrics
- ✅ **Scalability** via streaming channel-based design

Each stage is independent, testable, and can be configured separately for optimal performance with different Azure APIs.

