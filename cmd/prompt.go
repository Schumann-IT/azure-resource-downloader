package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"azure-resource-downloader/internal/logger"
)

// promptForDedicatedApp interactively requests the app registration client ID
// and tenant ID needed to download resource types that require a dedicated app
// (Microsoft Graph scopes the Azure CLI first-party app cannot provide). The
// requirements map (type -> declared permissions) is shown so the user knows
// which types drove the request.
//
// defaultClientID (typically from AZURE_RD_CLIENT_ID / config) and
// defaultTenantID (typically the Azure CLI session tenant) pre-fill each
// prompt: the user may press Enter to accept the shown default. It reads from
// in; when no input is available (e.g. a non-interactive run) it falls back to
// the defaults, and only errors when a required value has neither input nor a
// default.
func promptForDedicatedApp(requirements map[string][]string, in io.Reader, defaultClientID, defaultTenantID string) (clientID, tenantID string, err error) {
	log := logger.Default

	types := make([]string, 0, len(requirements))
	for t := range requirements {
		types = append(types, t)
	}
	sort.Strings(types)

	log.Warn("Some selected resource types need a dedicated app registration (device-code sign-in): the Azure CLI app cannot provide their Microsoft Graph permissions")
	for _, t := range types {
		if perms := requirements[t]; len(perms) > 0 {
			log.Warn("Requires dedicated app", "type", t, "permissions", strings.Join(perms, ", "))
		} else {
			log.Warn("Requires dedicated app", "type", t)
		}
	}
	fmt.Fprintln(os.Stderr, "Enter the app registration to sign in with (press Enter to accept a shown [default]).")
	fmt.Fprintln(os.Stderr, "To skip this prompt next time, pass --client-id/--tenant-id or export AZURE_RD_CLIENT_ID/AZURE_RD_TENANT_ID.")

	reader := bufio.NewReader(in)

	clientID, err = promptLine(reader, "Client ID", defaultClientID)
	if err != nil {
		return "", "", err
	}
	tenantID, err = promptLine(reader, "Tenant ID", defaultTenantID)
	if err != nil {
		return "", "", err
	}

	if clientID == "" || tenantID == "" {
		return "", "", errors.New("both client ID and tenant ID are required (pass --client-id/--tenant-id or set AZURE_RD_CLIENT_ID/AZURE_RD_TENANT_ID)")
	}
	return clientID, tenantID, nil
}

// promptLine writes label (with the default, if any) to stderr and reads a
// single trimmed line from reader. An empty response selects def. A closed
// input (EOF) with neither data nor a default is reported as an error so a
// non-interactive run fails fast rather than looping on empty input.
func promptLine(reader *bufio.Reader, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}

	line, err := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil {
		if errors.Is(err, io.EOF) {
			switch {
			case line != "":
				return line, nil
			case def != "":
				return def, nil
			default:
				return "", fmt.Errorf("no input available for %q (run interactively or pass --client-id/--tenant-id)", label)
			}
		}
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	if line == "" {
		return def, nil
	}
	return line, nil
}
