//go:build !((darwin && cgo) || linux || windows)

package azure

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// newPersistentCache reports that this build has no persistent token cache: the
// azidentity/cache module compiles only for darwin (with cgo), linux and
// windows. The device-code session then falls back to signing in on every run.
func newPersistentCache() (azidentity.Cache, error) {
	return azidentity.Cache{}, errors.New("persistent token cache unavailable on this platform/build")
}
