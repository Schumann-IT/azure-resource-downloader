package azure

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	betaodataerrors "github.com/microsoftgraph/msgraph-beta-sdk-go/models/odataerrors"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// jsonMessagePattern extracts the "Message" value from JSON error bodies that
// Microsoft Graph (Intune service) embeds in multi-line error strings.
var jsonMessagePattern = regexp.MustCompile(`(?i)"message"\s*:\s*"([^"]+)"`)

// permissionErrorMarkers are substrings that identify an authorization/permission
// failure across both ARM and Microsoft Graph APIs. They are matched
// case-insensitively as a fallback when a typed status code is unavailable
// (e.g. Microsoft Graph SDK errors).
var permissionErrorMarkers = []string{
	"authorizationfailed",
	"does not have authorization to perform action",
	"request authorization failed",
	"required scopes are missing",
	"authorization_requestdenied",
	"insufficient privileges",
	"forbidden",
	"is not authorized to perform this operation",
	"must have one of the following scopes",
}

// HTTPStatus returns the HTTP status code carried by a typed error anywhere in
// err's chain: an *azcore.ResponseError (ARM and every azcore-based client) or
// any error implementing Kiota's abstractions.ApiErrorable (the Microsoft Graph
// v1.0 and beta odataerrors.ODataError both embed abstractions.ApiError). The
// boolean is false for an untyped error, or a typed one without a status.
//
// A typed status is what lets callers tell a missing object (404) from a
// missing permission (401/403) without guessing from the message text.
func HTTPStatus(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode != 0 {
		return respErr.StatusCode, true
	}
	var apiErr abstractions.ApiErrorable
	if errors.As(err, &apiErr) && apiErr.GetStatusCode() != 0 {
		return apiErr.GetStatusCode(), true
	}
	return 0, false
}

// GraphErrorCode returns the OData error code (e.g. "Request_ResourceNotFound",
// "Authorization_RequestDenied") of a Microsoft Graph v1.0 or beta
// odataerrors.ODataError anywhere in err's chain, or "" when err carries none.
func GraphErrorCode(err error) string {
	var v1Err *odataerrors.ODataError
	if errors.As(err, &v1Err) {
		return mainErrorCode(v1Err.GetErrorEscaped())
	}
	var betaErr *betaodataerrors.ODataError
	if errors.As(err, &betaErr) {
		return mainErrorCode(betaErr.GetErrorEscaped())
	}
	return ""
}

// codeGetter is the part of the v1.0 and beta odataerrors.MainErrorable
// interfaces GraphErrorCode needs; both SDKs generate an identical GetCode.
type codeGetter interface {
	GetCode() *string
}

// mainErrorCode reads the code of an OData main error, tolerating a nil error
// object and a nil code.
func mainErrorCode(main codeGetter) string {
	if main == nil {
		return ""
	}
	if code := main.GetCode(); code != nil {
		return *code
	}
	return ""
}

// errorCode returns the service error code of a typed error: the ARM error
// code of an *azcore.ResponseError, else the OData code of a Graph error.
func errorCode(err error) string {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.ErrorCode != "" {
		return respErr.ErrorCode
	}
	return GraphErrorCode(err)
}

// isAuthStatus reports whether status is an authentication or authorization
// failure (401 or 403).
func isAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

// IsPermissionError reports whether err represents an authorization/permission
// failure, such as an ARM 403 (AuthorizationFailed) or a Microsoft Graph
// missing-scope / Forbidden error.
//
// A typed 401 or 403 (see HTTPStatus) is decisive; untyped errors fall back to
// matching known permission markers in the message text.
//
// When true, the signed-in user is simply not permitted to read the resource or
// resource type, so callers should warn and continue rather than treating it as
// a hard failure.
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	if status, ok := HTTPStatus(err); ok && isAuthStatus(status) {
		return true
	}

	msg := strings.ToLower(err.Error())
	for _, marker := range permissionErrorMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// hintMarker opens the "(hint: ...)" suffix handlers append to an error to name
// the permission a call needs.
const hintMarker = "(hint:"

// ErrorSummary returns a concise, single-line summary of err suitable for
// WARN-level log output and for the reason recorded for a type that could not
// be listed. It is deterministic for the same error. Multi-line details (HTTP
// response dumps, JSON error bodies) are stripped; callers should log the full
// error at debug level.
//
// A typed error (azcore or Microsoft Graph, see HTTPStatus) summarises as
// "HTTP <status>", then " <code>" when the service sent one, then
// ": <first line of the message>" — the handler's wrapping context included,
// its "(hint: ...)" removed. The handler's permission hint is appended only for
// a 401 or 403, so a 404 or a throttling error no longer blames a permission.
// An untyped error keeps its first line (plus the "Message" of an embedded JSON
// body when the rest is dropped) and gets the hint only when IsPermissionError
// recognises it.
func ErrorSummary(err error) string {
	if err == nil {
		return ""
	}

	full := err.Error()

	// The trailing "(hint: ...)" appended by handlers tells the user which
	// permission is missing — worth keeping only when a permission is missing.
	hint := ""
	if i := strings.LastIndex(full, hintMarker); i >= 0 {
		hint = strings.TrimSpace(full[i:])
	}

	if status, ok := HTTPStatus(err); ok {
		summary := fmt.Sprintf("HTTP %d", status)
		if code := errorCode(err); code != "" {
			summary += " " + code
		}
		if line := stripHint(firstLine(full)); line != "" {
			summary += ": " + line
		}
		if hint != "" && isAuthStatus(status) {
			summary += " " + hint
		}
		return summary
	}

	// Fall back to the first line of the error message. If the dropped
	// remainder is a JSON error body with a "Message" field (Intune-style
	// Graph errors), surface that message so the actual cause is not lost.
	line := firstLine(full)
	if line != full {
		if m := jsonMessagePattern.FindStringSubmatch(full); m != nil {
			line += ": " + strings.TrimSpace(m[1])
		}
	}
	line = stripHint(line)
	if hint != "" && IsPermissionError(err) {
		line += " " + hint
	}
	return line
}

// firstLine returns the first line of s, trimmed of the separators (" :{")
// that precede a dropped multi-line body. A single-line s is returned as-is.
func firstLine(s string) string {
	i := strings.IndexAny(s, "\r\n")
	if i < 0 {
		return s
	}
	return strings.TrimRight(strings.TrimSpace(s[:i]), " :{")
}

// stripHint removes a "(hint: ...)" suffix, and the space before it, from line.
func stripHint(line string) string {
	if i := strings.Index(line, hintMarker); i >= 0 {
		return strings.TrimSpace(line[:i])
	}
	return line
}
