package arm

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
)

// VirtualMachineHandler handles Azure Virtual Machines
type VirtualMachineHandler struct {
	credential     azcore.TokenCredential
	subscriptionID string
}

// NewVirtualMachineHandler creates a new virtual machine handler
func NewVirtualMachineHandler(credential azcore.TokenCredential, subscriptionID string) *VirtualMachineHandler {
	return &VirtualMachineHandler{
		credential:     credential,
		subscriptionID: subscriptionID,
	}
}

// GetType returns the Azure resource type
func (h *VirtualMachineHandler) GetType() string {
	return "Microsoft.Compute/virtualMachines"
}

// GetDocumentationPrompt returns the dedicated LLM documentation prompt for this resource type.
func (h *VirtualMachineHandler) GetDocumentationPrompt() string {
	return models.BuildDocumentationPrompt(h.Documentation())
}

// Documentation returns the per-type documentation metadata the prompt is built
// from. It satisfies models.Documented.
func (h *VirtualMachineHandler) Documentation() models.ResourceDocumentation {
	return models.ResourceDocumentation{
		Template:            armPromptTemplateText,
		AzureType:           h.GetType(),
		Purpose: "An Azure Virtual Machine with its size, OS profile, storage profile (image and OS disk) and network interfaces.",
		KeySettings: []string{
	"vmSize",
	"osProfile",
	"storageProfile.osDisk",
	"storageProfile.imageReference",
	"networkInterfaces",
},
		RequiredPermissions: []string{"Reader (Azure RBAC role on the subscription)"},
		Lifecycle: []string{
	"Deallocating stops compute billing but disks are still billed; a VM stopped from inside the guest OS stays allocated and keeps being billed.",
	"Deleting the VM can orphan NICs and disks unless their delete option (for example storageProfile.osDisk.deleteOption Delete) is set.",
	"Unmanaged disks were retired on 31 March 2026: a VM whose OS disk is not a managed disk can no longer be started.",
	"Keep OS patching and backup policies in place.",
},
		Links: models.ResourceLinks{
			EndpointDocs:  "https://learn.microsoft.com/en-us/rest/api/compute/virtual-machines",
			Permissions:   "https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/general#reader",
			BestPractices: []string{
		"https://learn.microsoft.com/en-us/azure/virtual-machines/security-policy",
		"https://learn.microsoft.com/en-us/azure/security/fundamentals/iaas",
	},
		},
	}
}

// List returns the IDs of all virtual machines in the subscription.
func (h *VirtualMachineHandler) List(ctx context.Context) ([]string, error) {
	return azure.ListResourcesByType(ctx, h.credential, h.subscriptionID, h.GetType())
}

// Fetch retrieves a virtual machine from Azure
func (h *VirtualMachineHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Parse resource ID
	idInfo, err := azure.ParseResourceID(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource ID: %w", err)
	}

	client, err := armcompute.NewVirtualMachinesClient(h.subscriptionID, h.credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtual machines client: %w", err)
	}

	resp, err := client.Get(ctx, idInfo.ResourceGroup, idInfo.ResourceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual machine: %w", err)
	}

	return resp.VirtualMachine, nil
}

// Transform converts the raw virtual machine into a cleaned version
func (h *VirtualMachineHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	vm, ok := resource.(armcompute.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected Virtual Machine")
	}

	if vm.Name == nil {
		return nil, fmt.Errorf("virtual machine name is nil")
	}

	properties := make(map[string]interface{})

	// Basic properties
	if vm.ID != nil {
		properties["id"] = *vm.ID
	}
	if vm.Name != nil {
		properties["name"] = *vm.Name
	}
	if vm.Location != nil {
		properties["location"] = *vm.Location
	}
	if vm.Type != nil {
		properties["type"] = *vm.Type
	}
	if len(vm.Tags) > 0 {
		properties["tags"] = vm.Tags
	}

	// VM Size
	if vm.Properties != nil && vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
		properties["vmSize"] = string(*vm.Properties.HardwareProfile.VMSize)
	}

	// OS Profile
	if vm.Properties != nil && vm.Properties.OSProfile != nil {
		osProfile := make(map[string]interface{})

		if vm.Properties.OSProfile.ComputerName != nil {
			osProfile["computerName"] = *vm.Properties.OSProfile.ComputerName
		}
		if vm.Properties.OSProfile.AdminUsername != nil {
			osProfile["adminUsername"] = *vm.Properties.OSProfile.AdminUsername
		}

		properties["osProfile"] = osProfile
	}

	// Storage Profile
	if vm.Properties != nil && vm.Properties.StorageProfile != nil {
		storageProfile := make(map[string]interface{})

		if vm.Properties.StorageProfile.ImageReference != nil {
			storageProfile["imageReference"] = vm.Properties.StorageProfile.ImageReference
		}
		if vm.Properties.StorageProfile.OSDisk != nil {
			storageProfile["osDisk"] = vm.Properties.StorageProfile.OSDisk
		}

		properties["storageProfile"] = storageProfile
	}

	// Network Profile
	if vm.Properties != nil && vm.Properties.NetworkProfile != nil && vm.Properties.NetworkProfile.NetworkInterfaces != nil {
		networkInterfaces := make([]interface{}, 0)
		for _, nic := range vm.Properties.NetworkProfile.NetworkInterfaces {
			if nic.ID != nil {
				networkInterfaces = append(networkInterfaces, map[string]interface{}{
					"id": *nic.ID,
				})
			}
		}
		if len(networkInterfaces) > 0 {
			properties["networkInterfaces"] = networkInterfaces
		}
	}

	return &models.TransformedResource{
		ID:          safeString(vm.ID),
		Type:        h.GetType(),
		Name:        safeString(vm.Name),
		DisplayName: safeString(vm.Name),
		Properties:  properties,
	}, nil
}
