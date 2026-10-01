//go:build (darwin && cgo) || linux || windows

package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"
)

// newPersistentCache builds the OS-protected token cache the device-code
// session keeps its tokens in: the login Keychain on macOS, a DPAPI-encrypted
// file on Windows, a file encrypted with a kernel-keyring key on Linux. This is
// the only file importing azidentity/cache, whose macOS accessor needs cgo; the
// build constraint keeps every other build compiling (tokencache_other.go).
// cache.New round-trips test data once and errors when no secure store works,
// so tokens are never stored unencrypted.
func newPersistentCache() (azidentity.Cache, error) {
	c, err := cache.New(&cache.Options{Name: tokenCacheName})
	if err != nil {
		return azidentity.Cache{}, fmt.Errorf("no secure token store: %w", err)
	}
	return c, nil
}
