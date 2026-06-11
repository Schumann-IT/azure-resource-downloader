package azure

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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

// IsPermissionError reports whether err represents an authorization/permission
// failure, such as an ARM 403 (AuthorizationFailed) or a Microsoft Graph
// missing-scope / Forbidden error.
//
// When true, the signed-in user is simply not permitted to read the resource or
// resource type, so callers should warn and continue rather than treating it as
// a hard failure.
func IsPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// ARM (and any azcore-based) errors expose a typed HTTP status code.
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		if respErr.StatusCode == http.StatusForbidden || respErr.StatusCode == http.StatusUnauthorized {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	for _, marker := range permissionErrorMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// ErrorSummary returns a concise, single-line summary of err suitable for
// WARN-level log output. Multi-line details (HTTP response dumps, JSON error
// bodies) are stripped; callers should log the full error at debug level.
func ErrorSummary(err error) string {
	if err == nil {
		return ""
	}

	full := err.Error()

	// Preserve a trailing "(hint: ...)" appended by handlers; it tells the
	// user which permission is missing.
	hint := ""
	if i := strings.LastIndex(full, "(hint:"); i >= 0 {
		hint = strings.TrimSpace(full[i:])
	}

	// ARM (and any azcore-based) errors carry a typed status and error code.
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		summary := fmt.Sprintf("HTTP %d", respErr.StatusCode)
		if respErr.ErrorCode != "" {
			summary += " " + respErr.ErrorCode
		}
		if hint != "" {
			summary += " " + hint
		}
		return summary
	}

	// Fall back to the first line of the error message. If the dropped
	// remainder is a JSON error body with a "Message" field (Intune-style
	// Graph errors), surface that message so the actual cause is not lost.
	line := full
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = strings.TrimRight(strings.TrimSpace(line[:i]), " :{")
		if m := jsonMessagePattern.FindStringSubmatch(full); m != nil {
			line += ": " + strings.TrimSpace(m[1])
		}
	}
	if hint != "" && !strings.Contains(line, "(hint:") {
		line += " " + hint
	}
	return line
}
