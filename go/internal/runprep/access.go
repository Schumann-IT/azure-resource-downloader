package runprep

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// ErrAccessRefused is returned when the access check that runs before a
// listing finds one or more permission groups the signed-in account may not
// read. Listing anyway would collect a refusal per type and end in a mostly
// empty, incomplete run, so the run is refused before anything is listed or
// written.
var ErrAccessRefused = errors.New("access check refused")

// AzureRBACGroup is the permission group of every Azure Resource Manager type:
// ARM reads are authorised by one RBAC role on the subscription, not by
// per-type delegated scopes.
const AzureRBACGroup = "AzureRBAC"

// deviceManagementGroupPrefix opens the name of every Intune permission group.
// A refusal there is an Intune role problem, which the service does not name.
const deviceManagementGroupPrefix = "DeviceManagement"

// PermissionGroup returns the permission group a delegated Microsoft Graph
// permission belongs to: the permission without its ".Read.All" or
// ".ReadWrite.All" suffix, so the read and the read-write variant of one
// permission (resolve-secrets asks for the latter) fall into one group.
func PermissionGroup(perm string) string {
	for _, suffix := range []string{".ReadWrite.All", ".Read.All"} {
		if strings.HasSuffix(perm, suffix) {
			return strings.TrimSuffix(perm, suffix)
		}
	}
	return perm
}

// TypeInfo is what the probe plan needs to know about one selected type.
type TypeInfo struct {
	// Type is the registered resource type name.
	Type string
	// Group is the type's permission group ("" when it is not probed).
	Group string
	// HasAccessProbe reports whether the type's handler has a dedicated
	// cheap probe (see models.AccessProber).
	HasAccessProbe bool
}

// Probe is one planned access probe: one request standing in for every
// selected type of a permission group.
type Probe struct {
	// Group is the permission group the probe answers for.
	Group string
	// ProbeType is the resource type whose handler is probed.
	ProbeType string
	// Blocks lists every selected type of the group, sorted: the types a
	// refusal of this probe blocks.
	Blocks []string
}

// PlanProbes returns one probe per permission group of types, sorted by group.
// Each group is probed through the first type (in sorted order) that has a
// dedicated access probe, else through its first type. Types without a group
// are not probed. It does no I/O.
func PlanProbes(types []TypeInfo) []Probe {
	byGroup := map[string][]TypeInfo{}
	for _, t := range types {
		if t.Group == "" {
			continue
		}
		byGroup[t.Group] = append(byGroup[t.Group], t)
	}

	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	probes := make([]Probe, 0, len(groups))
	for _, g := range groups {
		members := byGroup[g]
		sort.Slice(members, func(i, j int) bool { return members[i].Type < members[j].Type })
		probe := Probe{Group: g, ProbeType: members[0].Type}
		for _, m := range members {
			if m.HasAccessProbe {
				probe.ProbeType = m.Type
				break
			}
		}
		for _, m := range members {
			probe.Blocks = append(probe.Blocks, m.Type)
		}
		probes = append(probes, probe)
	}
	return probes
}

// TypeInfos describes the selected types for PlanProbes: an ARM type belongs
// to AzureRBACGroup (and is left out entirely without a subscription, because
// the listing skips it without a network call); a Microsoft Graph type to the
// group of its first declared permission; a type with neither, or without a
// registered handler, has no group.
func TypeInfos(registry *handlers.Registry, types []string, subscription string) []TypeInfo {
	infos := make([]TypeInfo, 0, len(types))
	for _, t := range types {
		if models.DetectAPIType(t) == models.APIAzureResourceManager && subscription == "" {
			continue
		}
		handler, err := registry.Get(t)
		if err != nil {
			continue
		}
		info := TypeInfo{Type: t}
		if models.DetectAPIType(t) == models.APIAzureResourceManager {
			info.Group = AzureRBACGroup
		} else if scoped, ok := handler.(models.PermissionScoped); ok && len(scoped.RequiredPermissions()) > 0 {
			info.Group = PermissionGroup(scoped.RequiredPermissions()[0])
		}
		if prober, ok := handler.(models.AccessProber); ok {
			info.HasAccessProbe = prober.HasAccessProbe()
		}
		infos = append(infos, info)
	}
	return infos
}

// PlanRunProbes plans the access check of a run: nothing for a --resource-id
// or --resource-group run (it lists nothing, so there is nothing to protect),
// else one probe per permission group of the effective selection.
func PlanRunProbes(registry *handlers.Registry, effective []string, subscription string, resourceIDs []string, resourceGroup string) []Probe {
	if len(resourceIDs) > 0 || resourceGroup != "" {
		return nil
	}
	return PlanProbes(TypeInfos(registry, effective, subscription))
}

// ProbeOutcome classifies the result of one access probe.
type ProbeOutcome int

const (
	// ProbeAllowed means the probe succeeded.
	ProbeAllowed ProbeOutcome = iota
	// ProbeRefused means the service answered 401 or 403.
	ProbeRefused
	// ProbeSignInFailed means the credential could not produce a token; every
	// listing would fail the same way, so it refuses like ProbeRefused.
	ProbeSignInFailed
	// ProbeInconclusive is every other failure (404, 400, throttling, 5xx, a
	// probe timeout, an untyped error): left to the listing, as before.
	ProbeInconclusive
	// ProbeCancelled means the run was interrupted.
	ProbeCancelled
)

// Refuses reports whether the outcome refuses the run.
func (o ProbeOutcome) Refuses() bool {
	return o == ProbeRefused || o == ProbeSignInFailed
}

// credentialUnavailableType is the dynamic type of azidentity's
// credential-unavailable error. The type is unexported, so it is matched by
// identity against an instance the package hands out.
var credentialUnavailableType = reflect.TypeOf(azidentity.NewCredentialUnavailableError(""))

// ClassifyProbe classifies a probe's error by its type only: a typed 401 or
// 403 refuses; an azidentity credential failure refuses as a failed sign-in;
// context cancellation is ProbeCancelled; everything else, including a probe
// deadline and any untyped error, is inconclusive. It deliberately matches no
// text: an Intune error body's Activity ID or URL can contain "403" or "429".
func ClassifyProbe(err error) ProbeOutcome {
	switch {
	case err == nil:
		return ProbeAllowed
	case errors.Is(err, context.Canceled):
		return ProbeCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return ProbeInconclusive
	case isSignInFailure(err):
		return ProbeSignInFailed
	}
	if status, ok := azure.HTTPStatus(err); ok && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		return ProbeRefused
	}
	return ProbeInconclusive
}

// isSignInFailure reports whether an azidentity credential failure sits
// anywhere in err's chain.
func isSignInFailure(err error) bool {
	var failed *azidentity.AuthenticationFailedError
	var required *azidentity.AuthenticationRequiredError
	if errors.As(err, &failed) || errors.As(err, &required) {
		return true
	}
	return chainContains(err, func(e error) bool { return reflect.TypeOf(e) == credentialUnavailableType })
}

// chainContains walks err's chain (both Unwrap forms) and reports whether any
// error in it satisfies match.
func chainContains(err error, match func(error) bool) bool {
	if err == nil {
		return false
	}
	if match(err) {
		return true
	}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		return chainContains(u.Unwrap(), match)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if chainContains(e, match) {
				return true
			}
		}
	}
	return false
}

// probeResult is one probe's slot in CheckAccess.
type probeResult struct {
	outcome ProbeOutcome
	err     error
}

// CheckAccess runs the planned probes concurrently (at most concurrency at a
// time, each bounded by timeout when it is positive) and refuses the run when
// any permission group is refused: it logs one error per refused group, in
// group order, and returns an error wrapping ErrAccessRefused. Inconclusive
// probes are logged at debug and do not refuse. An interrupted run returns the
// context's error. subscription names the subscription in the Azure RBAC hint.
func CheckAccess(ctx context.Context, registry *handlers.Registry, probes []Probe, subscription string, timeout time.Duration, concurrency int) error {
	if len(probes) == 0 {
		return nil
	}
	log := logger.Default
	log.Info("Checking access before listing", "permission_groups", len(probes))

	results := runProbes(ctx, registry, probes, timeout, concurrency)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("access check interrupted: %w", err)
	}

	refused := 0
	for i, r := range results {
		p := probes[i]
		switch {
		case r.outcome == ProbeCancelled:
			return fmt.Errorf("access check interrupted: %w", r.err)
		case r.outcome == ProbeInconclusive:
			log.Debug("Access check inconclusive; the listing decides", "group", p.Group, "probed_type", p.ProbeType,
				"reason", azure.ErrorSummary(r.err), "error", r.err)
		case r.outcome.Refuses():
			refused++
			logRefusal(p, r, subscription)
		}
	}
	if refused > 0 {
		return fmt.Errorf("%w %d of %d permission groups; narrow the run with --type, the type: list or exclude-type in the tenant profile",
			ErrAccessRefused, refused, len(probes))
	}
	log.Info("Access check passed", "permission_groups", len(probes))
	return nil
}

// runProbes runs every probe and returns its result in its own slot, so the
// order is the plan's regardless of completion order and no state is shared.
func runProbes(ctx context.Context, registry *handlers.Registry, probes []Probe, timeout time.Duration, concurrency int) []probeResult {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]probeResult, len(probes))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(i int, p Probe) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = probeResult{outcome: ProbeCancelled, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			err := probeOne(ctx, registry, p.ProbeType, timeout)
			results[i] = probeResult{outcome: ClassifyProbe(err), err: err}
		}(i, p)
	}
	wg.Wait()
	return results
}

// probeOne probes one type: its dedicated probe when the handler has the
// interface, else its listing (result discarded).
func probeOne(ctx context.Context, registry *handlers.Registry, resourceType string, timeout time.Duration) error {
	handler, err := registry.Get(resourceType)
	if err != nil {
		return err
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if prober, ok := handler.(models.AccessProber); ok {
		return prober.ProbeAccess(ctx)
	}
	_, err = handler.List(ctx)
	return err
}

// logRefusal logs one refused permission group: what was probed, what the
// service said, which selected types it blocks and what to do about it. The
// full error (Activity ID, URL) goes to debug only.
func logRefusal(p Probe, r probeResult, subscription string) {
	log := logger.Default
	kv := []interface{}{"group", p.Group, "probed_type", p.ProbeType}
	if r.outcome == ProbeSignInFailed {
		kv = append(kv, "status", "sign-in failed")
	} else {
		status, _ := azure.HTTPStatus(r.err)
		kv = append(kv, "status", status, "code", azure.ServiceErrorCode(r.err))
	}
	kv = append(kv, "blocks", p.Blocks, "hint", refusalHint(p.Group, r, subscription))
	log.Error("Access refused", kv...)
	log.Debug("Access probe refused", "group", p.Group, "probed_type", p.ProbeType, "error", r.err)
}

// refusalHint says what a refusal most likely means for the operator.
func refusalHint(group string, r probeResult, subscription string) string {
	switch {
	case r.outcome == ProbeSignInFailed:
		return "sign-in failed: " + azure.ErrorSummary(r.err)
	case group == AzureRBACGroup:
		return fmt.Sprintf("no Azure RBAC Reader role on subscription %s", subscription)
	case strings.HasPrefix(group, deviceManagementGroupPrefix):
		return "no Intune role for this account (a PIM-activated role may have expired)"
	}
	if msg := azure.GraphErrorMessage(r.err); msg != "" {
		return msg
	}
	return azure.ErrorSummary(r.err)
}
