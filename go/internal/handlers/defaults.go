package handlers

import (
	"azure-resource-downloader/internal/handlers/arm"
	"azure-resource-downloader/internal/handlers/graph"
	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// registerDefaults registers every supported resource handler on the given
// registry. ARM handlers are scoped to subscriptionID; Microsoft Graph handlers
// are tenant-level. resolveSecrets toggles Intune OMA-URI secret resolution in
// the device configuration handler.
//
// Handler construction is best-effort: a Graph handler whose client cannot be
// built (e.g. missing credential) is skipped rather than aborting registration.
func registerDefaults(r *Registry, cred azcore.TokenCredential, subscriptionID string, resolveSecrets bool) {
	// ARM handlers (scoped to the subscription).
	r.Register("Microsoft.Resources/resourceGroups", arm.NewResourceGroupHandler(cred, subscriptionID))
	r.Register("Microsoft.Storage/storageAccounts", arm.NewStorageAccountHandler(cred, subscriptionID))
	r.Register("Microsoft.Compute/virtualMachines", arm.NewVirtualMachineHandler(cred, subscriptionID))

	// Register the Intune device configuration handler separately because of
	// its extra resolve-secrets option.
	if resolveSecrets {
		logger.Default.Info("Secret resolution enabled", "flag", "--resolve-secrets")
		logger.Default.Debug("Secret resolution writes encrypted Intune OMA-URI values to output in plaintext; the signed-in user must hold delegated DeviceManagementConfiguration.ReadWrite.All and Intune read rights")
	}
	if dcHandler, err := graph.NewDeviceConfigurationHandler(cred, resolveSecrets); err == nil {
		r.Register(dcHandler.GetType(), dcHandler)
	}

	// Microsoft Graph collection handlers (tenant-level resources).
	graphCollections := []func(azcore.TokenCredential) (*graph.GraphCollectionHandler, error){
		graph.NewConditionalAccessPolicyHandler,
		graph.NewAuthenticationStrengthPolicyHandler,
		graph.NewDeviceManagementConfigurationPolicyHandler,
		graph.NewDeviceCompliancePolicyHandler,
		graph.NewCompliancePolicyHandler,
		graph.NewGroupPolicyConfigurationHandler,
		graph.NewDeviceManagementIntentHandler,
		graph.NewMobileAppHandler,
		graph.NewIosManagedAppProtectionHandler,
		graph.NewAndroidManagedAppProtectionHandler,
		graph.NewWindowsManagedAppProtectionHandler,
		graph.NewMdmWindowsInformationProtectionPolicyHandler,
		graph.NewWindowsInformationProtectionPolicyHandler,
		graph.NewMobileAppConfigurationHandler,
		graph.NewTargetedManagedAppConfigurationHandler,
		graph.NewWindowsAutopilotDeploymentProfileHandler,
		graph.NewWindowsAutopilotDeviceIdentityHandler,
		graph.NewDeviceEnrollmentConfigurationHandler,
		graph.NewApplePushNotificationCertificateHandler,
		graph.NewDepOnboardingSettingHandler,
		graph.NewAppleUserInitiatedEnrollmentProfileHandler,
		graph.NewRoleDefinitionHandler,
		graph.NewDeviceManagementSettingsHandler,
		graph.NewAuthenticationMethodsPolicyHandler,
		graph.NewAuthorizationPolicyHandler,
		graph.NewOnPremisesSynchronizationHandler,
		graph.NewOrganizationHandler,
		graph.NewOrganizationalBrandingHandler,
		graph.NewGroupHandler,
		graph.NewAssignmentFilterHandler,
		graph.NewWindowsFeatureUpdateProfileHandler,
		graph.NewWindowsQualityUpdateProfileHandler,
		graph.NewWindowsDriverUpdateProfileHandler,
		graph.NewDeviceCategoryHandler,
		graph.NewRoleScopeTagHandler,
		graph.NewTermsAndConditionsHandler,
		graph.NewIntuneBrandingProfileHandler,
		graph.NewNotificationMessageTemplateHandler,
		graph.NewNamedLocationHandler,
		graph.NewTermsOfUseAgreementHandler,
		graph.NewWindowsPlatformScriptHandler,
		graph.NewMacOSShellScriptHandler,
		graph.NewMacOSCustomAttributeScriptHandler,
		graph.NewWindowsRemediationScriptHandler,
		graph.NewDeviceComplianceScriptHandler,
		graph.NewReusablePolicySettingHandler,
		graph.NewVppTokenHandler,
		graph.NewMobileThreatDefenseConnectorHandler,
		graph.NewNdesConnectorHandler,
	}
	for _, newHandler := range graphCollections {
		if h, err := newHandler(cred); err == nil {
			r.Register(h.GetType(), h)
		}
	}

	// Add more handlers here as needed:
	// r.Register("Microsoft.Network/virtualNetworks", arm.NewVirtualNetworkHandler(cred, subscriptionID))
	// r.Register("Microsoft.Sql/servers", arm.NewSqlServerHandler(cred, subscriptionID))
}
