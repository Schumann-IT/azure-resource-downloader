package azure

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// userConfigDir resolves the per-user configuration directory the
// authentication records live under. It is a variable so tests point it at a
// temp directory and never touch the real one.
var userConfigDir = os.UserConfigDir

// authRecordPath returns where the authentication record of the device-code
// session for this tenant and app registration is kept:
// <user config dir>/azure-rd/auth/<tenant-id>-<client-id>.json. It is
// deliberately outside the repository, the --config-dir and the export tree:
// the record names an account, not a tenant's configuration.
func authRecordPath(tenantID, clientID string) (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the user configuration directory: %w", err)
	}
	return filepath.Join(dir, "azure-rd", "auth", tenantID+"-"+clientID+".json"), nil
}

// loadAuthRecord returns the stored authentication record for this tenant and
// app registration, and whether one was found. A missing, unreadable or
// unparsable file, or one whose tenant or client differs from the profile
// (compared case-insensitively), counts as absent: it is never trusted, and the
// next sign-in overwrites it.
func loadAuthRecord(tenantID, clientID string) (azidentity.AuthenticationRecord, bool) {
	path, err := authRecordPath(tenantID, clientID)
	if err != nil {
		return azidentity.AuthenticationRecord{}, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is built from the profile's ids under the user config dir
	if err != nil {
		return azidentity.AuthenticationRecord{}, false
	}
	var record azidentity.AuthenticationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return azidentity.AuthenticationRecord{}, false
	}
	if !strings.EqualFold(record.TenantID, tenantID) || !strings.EqualFold(record.ClientID, clientID) {
		return azidentity.AuthenticationRecord{}, false
	}
	return record, true
}

// saveAuthRecord writes the authentication record for this tenant and app
// registration, replacing any earlier one: the directory is created 0700, the
// file written 0600 through a temp file and a rename, so a crash never leaves a
// half-written record. The record holds no token or secret.
func saveAuthRecord(tenantID, clientID string, record azidentity.AuthenticationRecord) error {
	path, err := authRecordPath(tenantID, clientID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode the authentication record: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".record-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to write the authentication record: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to restrict the authentication record: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write the authentication record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write the authentication record: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to store the authentication record at %s: %w", path, err)
	}
	return nil
}

// authRecordInfo reports whether a usable record exists and when it was last
// written, for the --debug token-cache line.
func authRecordInfo(tenantID, clientID string) (fs.FileInfo, bool) {
	if _, ok := loadAuthRecord(tenantID, clientID); !ok {
		return nil, false
	}
	path, err := authRecordPath(tenantID, clientID)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}
