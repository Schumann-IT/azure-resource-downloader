package azure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// useConfigDir points the authentication record store at a temp directory for
// one test, so no test ever reads or writes the real user configuration.
func useConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = orig })
	return dir
}

// testRecord is a syntactically valid authentication record (version "1.0" is
// the one azidentity accepts); it carries no token.
func testRecord(tenantID, clientID string) azidentity.AuthenticationRecord {
	return azidentity.AuthenticationRecord{
		Authority:     "login.microsoftonline.com",
		ClientID:      clientID,
		HomeAccountID: "object.tenant",
		TenantID:      tenantID,
		Username:      "alice@contoso.com",
		Version:       "1.0",
	}
}

func TestAuthRecordPath(t *testing.T) {
	dir := useConfigDir(t)
	tests := []struct {
		name, tenant, client string
		want                 string
		wantErr              bool
	}{
		{name: "plain ids", tenant: "tenant-1", client: "client-1", want: filepath.Join(dir, "azure-rd", "auth", "tenant-1-client-1.json")},
		{name: "traversal in client id", tenant: "tenant-1", client: "../x", wantErr: true},
		{name: "separator in tenant id", tenant: "a/b", client: "client-1", wantErr: true},
		{name: "empty tenant id", tenant: "", client: "client-1", wantErr: true},
		{name: "empty client id", tenant: "tenant-1", client: "", wantErr: true},
		{name: "space in id", tenant: "tenant 1", client: "client-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := authRecordPath(tt.tenant, tt.client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("authRecordPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("authRecordPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthRecordRoundTrip(t *testing.T) {
	useConfigDir(t)
	record := testRecord("tenant-1", "client-1")
	if err := saveAuthRecord("tenant-1", "client-1", record); err != nil {
		t.Fatalf("saveAuthRecord() error = %v", err)
	}
	got, ok := loadAuthRecord("tenant-1", "client-1")
	if !ok {
		t.Fatal("loadAuthRecord() found no record after saving one")
	}
	if got != record {
		t.Errorf("loadAuthRecord() = %+v, want %+v", got, record)
	}

	// A later sign-in replaces the record.
	replaced := record
	replaced.Username = "bob@contoso.com"
	if err := saveAuthRecord("tenant-1", "client-1", replaced); err != nil {
		t.Fatalf("saveAuthRecord() replace error = %v", err)
	}
	if got, _ := loadAuthRecord("tenant-1", "client-1"); got.Username != "bob@contoso.com" {
		t.Errorf("record not replaced: username = %q", got.Username)
	}
}

// TestAuthRecordPermissions guards that the record and its directory are
// private to the user: the record names an account.
func TestAuthRecordPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	useConfigDir(t)
	if err := saveAuthRecord("tenant-1", "client-1", testRecord("tenant-1", "client-1")); err != nil {
		t.Fatalf("saveAuthRecord() error = %v", err)
	}
	path, _ := authRecordPath("tenant-1", "client-1")

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat record: %v", err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0o600 {
		t.Errorf("record mode = %o, want 600", mode)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat record dir: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("record directory mode = %o, want 700", mode)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read record dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("record directory holds %d entries, want only the record (no temp file left)", len(entries))
	}
}

// TestLoadAuthRecordRejects guards that a record that does not belong to the
// profile, or cannot be read, is treated as absent and never trusted.
func TestLoadAuthRecordRejects(t *testing.T) {
	tests := []struct {
		name    string
		content func(t *testing.T) []byte
	}{
		{name: "tenant mismatch", content: func(t *testing.T) []byte {
			return marshalRecord(t, testRecord("other-tenant", "client-1"))
		}},
		{name: "client mismatch", content: func(t *testing.T) []byte {
			return marshalRecord(t, testRecord("tenant-1", "other-client"))
		}},
		{name: "corrupt file", content: func(*testing.T) []byte { return []byte("{not json") }},
		{name: "unsupported version", content: func(*testing.T) []byte {
			return []byte(`{"tenantId":"tenant-1","clientId":"client-1","version":"42"}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useConfigDir(t)
			path, _ := authRecordPath("tenant-1", "client-1")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, tt.content(t), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, ok := loadAuthRecord("tenant-1", "client-1"); ok {
				t.Error("loadAuthRecord() trusted a record that must be treated as absent")
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		useConfigDir(t)
		if _, ok := loadAuthRecord("tenant-1", "client-1"); ok {
			t.Error("loadAuthRecord() found a record where none exists")
		}
	})

	t.Run("ids match case-insensitively", func(t *testing.T) {
		useConfigDir(t)
		if err := saveAuthRecord("tenant-1", "client-1", testRecord("TENANT-1", "Client-1")); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadAuthRecord("tenant-1", "client-1"); !ok {
			t.Error("loadAuthRecord() rejected a record differing only in case")
		}
	})
}

func marshalRecord(t *testing.T, record azidentity.AuthenticationRecord) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
