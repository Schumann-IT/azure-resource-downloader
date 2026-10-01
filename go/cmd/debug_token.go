package cmd

import (
	"context"
	"sort"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/runprep"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/charmbracelet/log"
	"github.com/spf13/viper"
)

// permissionStatus is one declared delegated permission of a selected resource
// type, and whether the session's Graph token carries it.
type permissionStatus struct {
	Type       string
	Permission string
	Covered    bool
}

// graphTokenReport is the --debug "Graph token" section: which application the
// session's Microsoft Graph token was issued to, the scopes it carries, and for
// every selected type that needs a dedicated app each declared permission
// marked covered or missing.
type graphTokenReport struct {
	AppID          string
	AppDisplayName string
	Scopes         []string
	Permissions    []permissionStatus
}

// buildGraphTokenReport assembles the Graph token section from the decoded
// token claims and the selected types' declared permissions (type → permissions,
// as DedicatedAppRequirements returns them). It is pure; the output is sorted by
// type, then by permission, so the section reads the same on every run.
func buildGraphTokenReport(claims azure.TokenClaims, requirements map[string][]string) graphTokenReport {
	report := graphTokenReport{
		AppID:          claims.AppID,
		AppDisplayName: claims.AppDisplayName,
		Scopes:         append([]string(nil), claims.Scopes...),
	}
	sort.Strings(report.Scopes)

	types := make([]string, 0, len(requirements))
	for resourceType := range requirements {
		types = append(types, resourceType)
	}
	sort.Strings(types)

	for _, resourceType := range types {
		covered, missing := azure.PermissionCoverage(requirements[resourceType], claims.Scopes)
		var rows []permissionStatus
		for _, permission := range covered {
			rows = append(rows, permissionStatus{Type: resourceType, Permission: permission, Covered: true})
		}
		for _, permission := range missing {
			rows = append(rows, permissionStatus{Type: resourceType, Permission: permission})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Permission < rows[j].Permission })
		report.Permissions = append(report.Permissions, rows...)
	}
	return report
}

// counts returns how many declared permissions are covered and missing.
func (r graphTokenReport) counts() (covered, missing int) {
	for _, p := range r.Permissions {
		if p.Covered {
			covered++
		} else {
			missing++
		}
	}
	return covered, missing
}

// logGraphTokenReport prints the Graph token section: the token line, then the
// permission coverage. It prints claims, never the token.
func logGraphTokenReport(logger *log.Logger, r graphTokenReport) {
	logGraphTokenLine(logger, r)
	logPermissionCoverage(logger, r)
}

// logGraphTokenLine prints which application the token was issued to and the
// sorted scopes it carries.
func logGraphTokenLine(logger *log.Logger, r graphTokenReport) {
	logger.Info("Graph token", "app_id", orNone(r.AppID), "app_display_name", orNone(r.AppDisplayName),
		"scp", orNone(strings.Join(r.Scopes, " ")))
}

// logPermissionCoverage prints every declared permission marked covered or
// missing, and the totals.
func logPermissionCoverage(logger *log.Logger, r graphTokenReport) {
	if len(r.Permissions) == 0 {
		logger.Info("Graph permission coverage", "result", "no selected type needs a dedicated app")
		return
	}
	for _, p := range r.Permissions {
		status := "missing"
		if p.Covered {
			status = "covered"
		}
		logger.Info("Declared permission", "type", p.Type, "permission", p.Permission, "status", status)
	}
	covered, missing := r.counts()
	logger.Info("Graph permission coverage", "covered", covered, "missing", missing)
}

// reportGraphToken fetches the session's Microsoft Graph token and prints the
// Graph token section for the effective type selection (the configured type
// list minus the tenant profile's exclusion), exactly as a download would
// select. Failures warn: the section is diagnostic and never fails --debug.
func reportGraphToken(ctx context.Context, logger *log.Logger, cred azcore.TokenCredential, registry *handlers.Registry) {
	claims, err := azure.GraphTokenClaims(ctx, cred)
	if err != nil {
		logger.Warn("Could not read the Microsoft Graph token", "reason", azure.ErrorSummary(err))
		logger.Debug("Graph token claims failed", "error", err)
		return
	}

	sel, err := runprep.SelectTypesFromConfig(registry.GetAllTypes(), viper.GetStringSlice("type"), nil, "")
	if err != nil {
		logGraphTokenLine(logger, buildGraphTokenReport(claims, nil))
		logger.Warn("Could not resolve the type selection; permission coverage omitted", "reason", err.Error())
		return
	}
	logGraphTokenReport(logger, buildGraphTokenReport(claims, registry.DedicatedAppRequirements(sel.Effective)))
}
