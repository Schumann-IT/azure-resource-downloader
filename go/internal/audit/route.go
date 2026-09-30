package audit

import (
	"regexp"
	"strings"

	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
)

// Reasons recorded for findings that are never queried.
const (
	reasonARM           = "ARM resource; AzureActivity is not consulted"
	reasonUnregistered  = "type not registered in this build"
	reasonNoTableMapped = "no audit table mapped for this type"
	reasonNoResourceID  = "the finding carries no resource id"
	reasonOutsideScope  = "outside this run's selection"
)

// intunePermissionPrefix marks a type whose changes Intune audits: any
// DeviceManagement* delegated permission routes the type to IntuneAuditLogs.
const intunePermissionPrefix = "DeviceManagement"

// entraPermissions are the delegated permissions whose types Entra ID audits
// into AuditLogs.
var entraPermissions = map[string]bool{
	"Policy.Read.All":                         true,
	"Group.Read.All":                          true,
	"Agreement.Read.All":                      true,
	"Organization.Read.All":                   true,
	"OrganizationalBranding.Read.All":         true,
	"OnPremDirectorySynchronization.Read.All": true,
}

// noJoinKeyTypes are the types whose resource id is no usable audit target:
// singletons and pseudo-ids (the tenant GUID, an Apple ID, a fixed label) and
// role scope tags, whose ids are numeric strings. The id guard below would
// reject most of them anyway; naming them makes the reason precise and keeps a
// singleton whose pseudo-id happens to be a GUID (the tenant id) from joining
// to unrelated tenant-level events.
var noJoinKeyTypes = map[string]string{
	"Microsoft.Graph/organization":                     "singleton; its id is the tenant id, not an audit target",
	"Microsoft.Graph/onPremisesSynchronization":        "singleton; its id is the tenant id, not an audit target",
	"Microsoft.Graph/authorizationPolicy":              "singleton; its pseudo-id is not an audit target",
	"Microsoft.Graph/authenticationMethodsPolicy":      "singleton; its pseudo-id is not an audit target",
	"Microsoft.Graph/deviceManagement":                 "singleton; its pseudo-id is not an audit target",
	"Microsoft.Graph/organizationalBranding":           "singleton; its pseudo-id is not an audit target",
	"Microsoft.Graph/applePushNotificationCertificate": "singleton; its id is an Apple ID, not an audit target",
	"Microsoft.Graph/roleScopeTags":                    "role scope tag ids are numeric, not audit target ids",
}

// joinIDPattern is the only character set a resource id may have to reach a
// query string. It is what makes interpolating ids into KQL safe: no quote,
// backslash, whitespace or operator can pass it.
var joinIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// guidPattern finds a GUID embedded in an id (T_<guid>, A_<guid>,
// <guid>_DefaultPlatformRestrictions) or forming all of it.
var guidPattern = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// router maps a finding to the audit table that records its changes. The type
// → table map is built once from the registry's declared permissions.
type router struct {
	tables     map[string]string
	registered map[string]bool
}

// newRouter builds the type → table map from what each registered handler
// already declares: models.DetectAPIType separates ARM from Graph, and a Graph
// handler's RequiredPermissions decide the table. An Intune permission wins
// over an Entra permission on the same type.
func newRouter(registry *handlers.Registry) *router {
	r := &router{tables: map[string]string{}, registered: map[string]bool{}}
	for _, t := range registry.GetAllTypes() {
		r.registered[t] = true
		if models.DetectAPIType(t) != models.APIMicrosoftGraph {
			continue
		}
		h, err := registry.Get(t)
		if err != nil {
			continue
		}
		scoped, ok := h.(models.PermissionScoped)
		if !ok {
			continue
		}
		if table := tableForPermissions(scoped.RequiredPermissions()); table != "" {
			r.tables[t] = table
		}
	}
	return r
}

// tableForPermissions decides the audit table from a type's delegated
// permissions, or "" when none maps.
func tableForPermissions(perms []string) string {
	table := ""
	for _, p := range perms {
		if strings.HasPrefix(p, intunePermissionPrefix) {
			return drift.TableIntuneAuditLogs
		}
		if entraPermissions[p] {
			table = drift.TableAuditLogs
		}
	}
	return table
}

// Route decides where a finding's attribution comes from. A finding that can
// be queried returns its table and an empty status; every other finding
// returns the status it ends with (not-queried or no-join-key) and the reason.
// The finding's type is taken from its key, exactly as the drift engine
// derives it.
func Route(registry *handlers.Registry, key string, f drift.Finding) (table, status, reason string) {
	return newRouter(registry).route(key, f)
}

// route is Route over a prebuilt map.
func (r *router) route(key string, f drift.Finding) (table, status, reason string) {
	rtype := drift.TypeOfKey(key)
	switch {
	case models.DetectAPIType(rtype) != models.APIMicrosoftGraph:
		return "", drift.AttributionNotQueried, reasonARM
	case !r.registered[rtype]:
		return "", drift.AttributionNotQueried, reasonUnregistered
	}
	table = r.tables[rtype]
	if table == "" {
		return "", drift.AttributionNotQueried, reasonNoTableMapped
	}
	if why, singleton := noJoinKeyTypes[rtype]; singleton {
		return table, drift.AttributionNoJoinKey, why
	}
	if len(joinIDs(f.ResourceID)) == 0 {
		if f.ResourceID == "" {
			return table, drift.AttributionNoJoinKey, reasonNoResourceID
		}
		return table, drift.AttributionNoJoinKey, "resource id " + quoteForReason(f.ResourceID) + " contains no GUID the audit tables record as a target"
	}
	return table, "", ""
}

// joinIDs returns the ids a resource id is queried and matched as: nothing
// when it fails the character-set guard or embeds no GUID, the id alone when
// it is a GUID, and the id plus its embedded GUID otherwise. All are
// lower-cased, since the join is case-insensitive.
func joinIDs(resourceID string) []string {
	if !joinIDPattern.MatchString(resourceID) {
		return nil
	}
	guid := guidPattern.FindString(resourceID)
	if guid == "" {
		return nil
	}
	id := strings.ToLower(resourceID)
	guid = strings.ToLower(guid)
	if id == guid {
		return []string{id}
	}
	return []string{id, guid}
}

// quoteForReason renders an id inside a reason, truncated so a pathological id
// cannot bloat the artifact.
func quoteForReason(id string) string {
	const maxLen = 80
	if len(id) > maxLen {
		id = id[:maxLen] + "…"
	}
	return "\"" + id + "\""
}
