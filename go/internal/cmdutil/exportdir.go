package cmdutil

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/tenantdir"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/spf13/viper"
)

// ResolveExportDir decides which export directory an offline-capable command
// acts on, and the domain to cross-check its metadata against, through the
// resolver every command shares. An explicit declaredDomain runs offline;
// otherwise it signs in with cred (the credential the tenant's profile
// supplies) to resolve the tenant domain, and failing that — or with a nil
// cred, when the caller could not build one — falls back to the single export
// directory under baseOutput, which is then treated as a declaration, not as a
// confirmed tenant.
//
// Taking the credential rather than building one lets a caller sign in once
// and reuse the same credential afterwards (resource audit queries Log
// Analytics with it).
func ResolveExportDir(ctx context.Context, baseOutput, declaredDomain string, cred azcore.TokenCredential) (tenantDir, expectDomain string, err error) {
	domain := declaredDomain
	resolved := ""
	if domain == "" {
		resolved = resolveSignedInDomain(ctx, cred)
		if resolved == "" {
			d, derr := singleExportDomain(baseOutput)
			if derr != nil {
				return "", "", derr
			}
			logger.Default.Info("Defaulting to the only export directory found", "domain", d)
			domain = d
		}
	}

	target, err := tenantdir.Resolve(baseOutput, domain, resolved)
	if err != nil {
		return "", "", err
	}
	return target.Dir, target.Domain, nil
}

// resolveSignedInDomain returns the signed-in tenant's default domain, or ""
// (with a warning) when it cannot be resolved.
func resolveSignedInDomain(ctx context.Context, cred azcore.TokenCredential) string {
	log := logger.Default
	if cred == nil {
		log.Warn("Authentication failed; falling back to a single export directory (pass --domain to run offline)",
			"reason", "no credential could be built")
		return ""
	}
	client, err := azure.NewClientWithCredential(ctx, cred, viper.GetString("subscription"), viper.GetString("tenant-id"))
	if err != nil {
		log.Warn("Authentication failed; falling back to a single export directory (pass --domain to run offline)",
			"reason", azure.ErrorSummary(err))
		return ""
	}
	d, err := client.GetTenantDomain(ctx)
	if err != nil {
		log.Warn("Could not resolve tenant domain; falling back to a single export directory",
			"reason", azure.ErrorSummary(err))
		return ""
	}
	return d
}

// singleExportDomain returns the single tenant directory under baseOutput that
// holds an export. It refuses to guess when zero or several exist.
func singleExportDomain(baseOutput string) (string, error) {
	candidates := ExportDomains(baseOutput)
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return "", fmt.Errorf("no export directory under %q (expected <domain>/resources/metadata.yaml); pass --domain", baseOutput)
	default:
		return "", fmt.Errorf("several export directories under %q (%v); pass --domain to choose", baseOutput, candidates)
	}
}

// ProfileCredential builds the credential the loaded tenant profile names
// (client-id / tenant-id; the az login session when both are empty). It makes
// no network call. On failure it warns and returns nil, which
// ResolveExportDir treats as "could not sign in".
func ProfileCredential() azcore.TokenCredential {
	cred, err := azure.NewCredential(viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		logger.Default.Warn("Could not build a credential from the tenant profile", "reason", azure.ErrorSummary(err))
		return nil
	}
	return cred
}
