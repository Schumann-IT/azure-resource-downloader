# Pipeline Flow - Visual Guide

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        AZURE RESOURCE DOWNLOADER                             │
│                         Streaming Pipeline Architecture                      │
└─────────────────────────────────────────────────────────────────────────────┘

                                 INPUT
                                   ↓
                    ┌──────────────────────────┐
                    │   CLI / Configuration    │
                    │  - Resource IDs          │
                    │  - Resource Types        │
                    │  - Worker Count          │
                    │  - Import Format         │
                    └──────────────────────────┘
                                   ↓
                          [FetchRequest List]
                                   ↓
┌──────────────────────────────────────────────────────────────────────────────┐
│                          STAGE 1: FETCHER                                     │
│                   Retrieve Resources from Azure                               │
└──────────────────────────────────────────────────────────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   Worker Pool (N)        │
                    │  ┌────┐ ┌────┐ ┌────┐   │
                    │  │ W1 │ │ W2 │ │ WN │   │
                    │  └────┘ └────┘ └────┘   │
                    └──────────────────────────┘
                                   ↓
              Each Worker:
              1. Parse Resource ID → Extract Type
              2. Get Handler (Registry Lookup)
              3. Fetch from Azure API (with retry)
              4. Return FetchResult
                                   ↓
                      [Channel: FetchResult]
                      Streams results as fetched
                                   ↓
┌──────────────────────────────────────────────────────────────────────────────┐
│                       STAGE 2: TRANSFORMER                                    │
│                  Transform & Clean Resource Data                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   Worker Pool (N)        │
                    │  ┌────┐ ┌────┐ ┌────┐   │
                    │  │ W1 │ │ W2 │ │ WN │   │
                    │  └────┘ └────┘ └────┘   │
                    └──────────────────────────┘
                                   ↓
              Each Worker:
              1. Get Handler
              2. Transform Raw Data → Clean Map
              3. Apply Exclusion Rules
              4. Resolve Azure IDs → Names
              5. Sanitize Names
              6. Generate Terraform Import Block
              7. Return TransformResult
                                   ↓
                   [Channel: TransformResult]
                   Streams transformed data
                                   ↓
┌──────────────────────────────────────────────────────────────────────────────┐
│                          STAGE 3: WRITER                                      │
│                   Write Files & Collect Imports                               │
└──────────────────────────────────────────────────────────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   Worker Pool (N)        │
                    │  ┌────┐ ┌────┐ ┌────┐   │
                    │  │ W1 │ │ W2 │ │ WN │   │
                    │  └────┘ └────┘ └────┘   │
                    └──────────────────────────┘
                                   ↓
              Each Worker:
              1. Create Directory
              2. Write YAML File (resource data)
              3. Collect Import Statement (thread-safe)
              4. Return WriteResult
                                   ↓
                   [Channel: WriteResult]
                   Results available immediately
                                   ↓
                 ┌─────────────────┴──────────────────┐
                 │                                    │
                 ↓                                    ↓
        After All Workers:                    Main Thread:
        Write Consolidated                    Collect Results
        import.tf Files                       Track Progress
        (One per resource type)               Report Metrics
                 ↓                                    ↓
                 └─────────────────┬──────────────────┘
                                   ↓
                                OUTPUT
                                   ↓
                    ┌──────────────────────────┐
                    │  File System             │
                    │  - YAML files            │
                    │  - import.tf files       │
                    │  - Directory structure   │
                    └──────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │  Execution Summary       │
                    │  - Success count         │
                    │  - Failed count          │
                    │  - Errors list           │
                    │  - Performance metrics   │
                    └──────────────────────────┘
```

---

## Detailed Stage Breakdown

### STAGE 1: FETCHER - Resource Retrieval

```
Input: []*FetchRequest
    ↓
┌─────────────────────────────────────────┐
│  Parse Resource ID                      │
│  "/subs/XXX/rg/my-rg" → "my-rg"       │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Registry Lookup                        │
│  Type → Handler                         │
│  "Microsoft.Storage/*" → Handler        │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Azure API Call (with Retry)           │
│  ┌─────────────────────────────────┐   │
│  │ Attempt 1: Call Azure API       │   │
│  │ → Success? Return data          │   │
│  │ → Rate limit? Wait & retry      │   │
│  │ → Error? Backoff & retry        │   │
│  │                                  │   │
│  │ Retry Config:                   │   │
│  │ - Max: 5 attempts               │   │
│  │ - Backoff: 1s, 2s, 4s, 8s, 16s │   │
│  │ - Handles: 429, 500, 502-504   │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
    ↓
Output: FetchResult {
    ResourceID   string
    ResourceType string
    RawData      interface{} ← Azure SDK object
    Error        error
}
```

### STAGE 2: TRANSFORMER - Data Transformation

```
Input: FetchResult
    ↓
┌─────────────────────────────────────────┐
│  Handler Transform                      │
│  Raw Azure Object → Clean Map          │
│                                         │
│  Example:                               │
│  ResourceGroup SDK → {                 │
│    "id": "...",                        │
│    "name": "my-rg",                    │
│    "location": "eastus",               │
│    "tags": {...}                       │
│  }                                      │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Apply Exclusions                       │
│  ┌─────────────────────────────────┐   │
│  │ Global Keys:                    │   │
│  │ - provisioningState             │   │
│  │ - etag                          │   │
│  │ - systemData                    │   │
│  │                                  │   │
│  │ Type-Specific Keys:             │   │
│  │ - managedBy (resource groups)   │   │
│  │ - primaryEndpoints (storage)    │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Resolve Azure IDs                      │
│  "/subs/.../rg/my-rg" → "my-rg"        │
│  Improves Terraform readability         │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Sanitize Names                         │
│  "My-Resource@123!" → "my_resource_123" │
│  Valid for files & Terraform            │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Generate Terraform Import              │
│  ┌─────────────────────────────────┐   │
│  │ Template: {resource_type}.{name}│   │
│  │                                  │   │
│  │ Output:                         │   │
│  │ import {                        │   │
│  │   to = azurerm_resource_group.  │   │
│  │        my_rg                    │   │
│  │   id = "/subs/.../rg/my-rg"    │   │
│  │ }                               │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
    ↓
Output: TransformResult {
    ResourceID            string
    DisplayName           string
    SanitizedName         string
    CleanedData           map[string]interface{}
    TerraformImport       string
    TerraformResourceType string
    Error                 error
}
```

### STAGE 3: WRITER - File Generation

```
Input: TransformResult
    ↓
┌─────────────────────────────────────────┐
│  Create Directory Structure             │
│  output/Microsoft.Resources/            │
│         resourceGroups/                 │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Write YAML File                        │
│  File: my_rg.yaml                       │
│  ┌─────────────────────────────────┐   │
│  │ ---                             │   │
│  │ id: /subs/.../rg/my-rg         │   │
│  │ name: my-rg                     │   │
│  │ location: eastus                │   │
│  │ tags:                           │   │
│  │   environment: production       │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────┐
│  Collect Import (Thread-Safe)           │
│  ┌─────────────────────────────────┐   │
│  │ mutex.Lock()                    │   │
│  │ importsByType[type] = append    │   │
│  │ mutex.Unlock()                  │   │
│  │                                  │   │
│  │ Grouped by resource type        │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
    ↓
Output: WriteResult {
    ResourceID    string
    YAMLPath      string
    TerraformPath string
    Error         error
}
    ↓
┌─────────────────────────────────────────┐
│  After ALL Workers Complete:            │
│  ───────────────────────────────────    │
│  Write Consolidated import.tf           │
│  ┌─────────────────────────────────┐   │
│  │ For each resource type:         │   │
│  │                                  │   │
│  │ # Import for my-rg              │   │
│  │ import {                        │   │
│  │   to = azurerm_resource_group.  │   │
│  │        my_rg                    │   │
│  │   id = "..."                    │   │
│  │ }                               │   │
│  │                                  │   │
│  │ # Import for another-rg         │   │
│  │ import {                        │   │
│  │   to = azurerm_resource_group.  │   │
│  │        another_rg               │   │
│  │   id = "..."                    │   │
│  │ }                               │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

---

## Concurrency Timeline

```
TIME →

Fetch Stage:
  Worker 1: [Resource A] ────────→ Done
  Worker 2:      [Resource B] ──────────→ Done
  Worker 3:           [Resource C] ────────────→ Done

Transform Stage:
  Worker 1:    [Resource A] ──→ Done
  Worker 2:         [Resource B] ──────→ Done
  Worker 3:              [Resource C] ──────→ Done

Write Stage:
  Worker 1:       [Resource A] → Done
  Worker 2:            [Resource B] ────→ Done
  Worker 3:                 [Resource C] ────→ Done
  
  Then: Write consolidated import.tf files

Timeline:
  0s ───────── 2s ───────── 4s ───────── 6s ───────── 8s ───────── 10s
  │           │            │            │            │            │
  Start       │            │            │            │            All Complete
  All         Resource A   Resource B   Resource C   Last         Consolidated
  Stages      Complete     Complete     Complete     Worker       import.tf
                                                     Done         Written

Key: Resources flow through pipeline, don't wait for all fetches!
```

---

## Error Flow

```
┌─────────────────────────────────────────┐
│  FETCHER                                │
│  Error: API Failure                     │
└─────────────────────────────────────────┘
    ↓
FetchResult {
    ResourceID: "...",
    Error: "failed to fetch"
}
    ↓
┌─────────────────────────────────────────┐
│  TRANSFORMER                            │
│  Detects Error                          │
│  → Skip transform, pass through         │
└─────────────────────────────────────────┘
    ↓
TransformResult {
    ResourceID: "...",
    Error: "failed to fetch"  ← Same error
}
    ↓
┌─────────────────────────────────────────┐
│  WRITER                                 │
│  Detects Error                          │
│  → Skip write, return error result      │
└─────────────────────────────────────────┘
    ↓
WriteResult {
    ResourceID: "...",
    Error: "failed to fetch"  ← Same error
}
    ↓
┌─────────────────────────────────────────┐
│  SUMMARY                                │
│  FailedResources++                      │
│  Errors = append(error message)         │
└─────────────────────────────────────────┘

Other resources continue processing normally!
```

---

## Data Flow Example

### Example: Fetching Resource Group "my-rg"

```
Step 1: FETCH
─────────────
Input: FetchRequest {
    ResourceID: "/subscriptions/XXX/resourceGroups/my-rg"
    ResourceType: "Microsoft.Resources/resourceGroups"
}

↓ Parse ID
↓ Get ResourceGroupHandler
↓ Call Azure API

Output: FetchResult {
    ResourceID: "/subscriptions/XXX/resourceGroups/my-rg"
    ResourceType: "Microsoft.Resources/resourceGroups"
    RawData: &armresources.ResourceGroup{
        ID:       "/subscriptions/XXX/resourceGroups/my-rg",
        Name:     "my-rg",
        Location: "eastus",
        Tags: map[string]*string{
            "environment": "production",
        },
        Properties: &armresources.ResourceGroupProperties{
            ProvisioningState: "Succeeded",
        },
    }
}

────────────────────────────────────────────────────

Step 2: TRANSFORM
─────────────────
Input: FetchResult (from above)

↓ Handler.Transform() converts SDK object to map
↓ Apply exclusions (remove provisioningState)
↓ Resolve IDs
↓ Sanitize name: "my-rg" → "my_rg"
↓ Generate import block

Output: TransformResult {
    ResourceID: "/subscriptions/XXX/resourceGroups/my-rg"
    ResourceType: "Microsoft.Resources/resourceGroups"
    DisplayName: "my-rg"
    SanitizedName: "my_rg"
    CleanedData: {
        "id": "/subscriptions/XXX/resourceGroups/my-rg",
        "name": "my-rg",
        "location": "eastus",
        "tags": {
            "environment": "production",
        },
    }
    TerraformImport: "import {\n  to = azurerm_resource_group.my_rg\n  id = \"...\"\n}\n"
    TerraformResourceType: "azurerm_resource_group"
}

────────────────────────────────────────────────────

Step 3: WRITE
─────────────
Input: TransformResult (from above)

↓ Create directory: output/Microsoft.Resources/resourceGroups/
↓ Write YAML: my_rg.yaml
↓ Collect import statement

Output: WriteResult {
    ResourceID: "/subscriptions/XXX/resourceGroups/my-rg"
    YAMLPath: "output/Microsoft.Resources/resourceGroups/my_rg.yaml"
    TerraformPath: "output/Microsoft.Resources/resourceGroups/import.tf"
    Error: nil
}

↓ After all workers complete
↓ Write consolidated import.tf

Files Created:
──────────────
1. output/Microsoft.Resources/resourceGroups/my_rg.yaml
   ---
   id: /subscriptions/XXX/resourceGroups/my-rg
   name: my-rg
   location: eastus
   tags:
     environment: production

2. output/Microsoft.Resources/resourceGroups/import.tf
   # Terraform import statements
   # Generated by azure-resource-downloader
   
   # Import for my-rg
   import {
     to = azurerm_resource_group.my_rg
     id = "/subscriptions/XXX/resourceGroups/my-rg"
   }
```

---

## Performance Characteristics

### Worker Pool Benefits

```
Sequential Processing (1 worker):
───────────────────────────────────
Resource A: [Fetch 2s][Trans 1s][Write 0.5s]
Resource B:                                    [Fetch 2s][Trans 1s][Write 0.5s]
Resource C:                                                                       [Fetch 2s][Trans 1s][Write 0.5s]
Total Time: 10.5 seconds

Parallel Processing (3 workers per stage):
──────────────────────────────────────────
Resource A: [Fetch 2s][Trans 1s][Write 0.5s]
Resource B: [Fetch 2s][Trans 1s][Write 0.5s]
Resource C: [Fetch 2s][Trans 1s][Write 0.5s]
Total Time: 3.5 seconds (3x faster!)
```

### Channel Streaming Benefits

```
Without Streaming (batch processing):
─────────────────────────────────────
[Fetch All Resources] → [Transform All] → [Write All]
       10s                   5s              2.5s
Total: 17.5 seconds

With Streaming (pipeline):
─────────────────────────
[Fetch]────→[Transform]────→[Write]
  10s          5s              2.5s
Resources flow through as ready
Total: ~12 seconds (earliest resources done much sooner!)
```

---

This architecture provides optimal performance through:
- ✅ **Parallelism**: Multiple workers per stage
- ✅ **Streaming**: Resources flow through immediately
- ✅ **Independence**: Stages run concurrently
- ✅ **Resilience**: Errors don't block other resources
- ✅ **Scalability**: Configurable worker counts per API

