package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// profileExt is the extension a tenant profile file carries. Profiles are
// always <domain>.yaml so the file name is the tenant's domain and nothing else
// has to be parsed out of it.
const profileExt = ".yaml"

// ErrProfileMissing is returned when a config directory was given but holds no
// profile for the requested domain. It is deliberately fatal: falling back to
// the built-in defaults would run with the wrong (empty) configuration while
// looking like it worked.
var ErrProfileMissing = errors.New("no configuration profile for this domain")

// ErrDomainRequired is returned when a config directory was given without an
// explicit --domain. The tenant a run acts on is only known after
// authentication, and the profile is what supplies the credentials
// authentication needs, so the profile cannot be chosen by the resolved tenant.
var ErrDomainRequired = errors.New("--config-dir requires --domain")

// Options describes where to look for configuration.
type Options struct {
	// ConfigFile is an explicit base file (--config). It wins over the
	// base.yaml a config directory may hold.
	ConfigFile string
	// ConfigDir is a directory of per-tenant profiles (--config-dir), possibly
	// alongside a base.yaml.
	ConfigDir string
	// Domain is the tenant domain explicitly passed as --domain. Only an
	// explicit value selects a profile.
	Domain string
}

// Result reports which files were actually loaded, for the "Using config file"
// log line and the --debug report.
type Result struct {
	BaseFile    string
	ProfileFile string
	Domain      string
}

// Load resolves the base configuration and the tenant profile, validates each
// against the key partition, and merges them into v with the profile last. It
// reads no environment variables: configuration reaches the tool through these
// files and the few command-line flags only.
func Load(v *viper.Viper, opts Options) (Result, error) {
	var res Result

	SetDefaults(v)

	baseFile, profileFile, err := resolveFiles(opts)
	if err != nil {
		return res, err
	}
	if err := mergeFiles(v, baseFile, profileFile); err != nil {
		return res, err
	}
	if err := validateValues(v, profileFile); err != nil {
		return res, err
	}

	res.BaseFile = baseFile
	res.ProfileFile = profileFile
	if profileFile != "" {
		res.Domain = opts.Domain
	}
	return res, nil
}

// resolveFiles decides which files this run's configuration comes from and
// validates each against the key partition.
//
// Validation happens here, per file, and deliberately BEFORE any merging: the
// merged state no longer records which file a key came from, so validating it
// could neither name the offender nor catch a key that is legal in one file and
// illegal in the other.
func resolveFiles(opts Options) (baseFile, profileFile string, err error) {
	if opts.ConfigDir != "" && opts.Domain == "" {
		return "", "", fmt.Errorf("%w: the tenant is not known until after authentication, and the profile supplies the credentials authentication needs", ErrDomainRequired)
	}

	if baseFile, err = resolveBaseFile(opts); err != nil {
		return "", "", err
	}
	if profileFile, err = resolveProfileFile(opts); err != nil {
		return "", "", err
	}

	for _, f := range []struct {
		path  string
		scope Scope
	}{
		{path: baseFile, scope: ScopeGeneral},
		{path: profileFile, scope: ScopeTenant},
	} {
		if f.path == "" {
			continue
		}
		if err := validateFile(f.path, f.scope); err != nil {
			return "", "", err
		}
	}
	return baseFile, profileFile, nil
}

// mergeFiles reads the base file and then merges the profile over it, so the
// tenant's own settings win. Either may be absent; with both absent the
// built-in defaults stand and the run is a zero-config run.
func mergeFiles(v *viper.Viper, baseFile, profileFile string) error {
	loaded := false
	for _, file := range []string{baseFile, profileFile} {
		if file == "" {
			continue
		}
		v.SetConfigFile(file)
		read := v.MergeInConfig
		if !loaded {
			read = v.ReadInConfig
		}
		if err := read(); err != nil {
			return fmt.Errorf("failed to read config file %q: %w", file, err)
		}
		loaded = true
	}
	return nil
}

// guidPattern is the shape of a Log Analytics workspace id.
var guidPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// validateValues checks the values whose shape the merged configuration can
// prove wrong before any command runs. audit-workspace-id must be a workspace
// id (GUID): a workspace name or a full ARM resource id would need a
// subscription and Reader rights to resolve, and a wrong workspace does not
// fail loudly — it returns no rows. The key is tenant-scoped, so a value can
// only have come from the profile.
//
// exclude-type must be a list of non-empty strings. Only the shape is checked
// here: whether each name is a registered type is decided where the registry
// is known (internal/runprep), which this package deliberately does not import.
func validateValues(v *viper.Viper, profileFile string) error {
	id := strings.TrimSpace(v.GetString(AuditWorkspaceKey))
	if id != "" && !guidPattern.MatchString(id) {
		return fmt.Errorf("invalid configuration in %s: %q must be a Log Analytics workspace id (a GUID), not a name or resource id: %q",
			profileFile, AuditWorkspaceKey, id)
	}
	if err := validateExcludeType(v.Get(ExcludeTypeKey)); err != nil {
		return fmt.Errorf("invalid configuration in %s: %q %w", profileFile, ExcludeTypeKey, err)
	}
	return nil
}

// validateExcludeType checks the shape of the exclude-type value: absent, or a
// list whose every item is a non-empty string. A scalar is refused rather than
// read as a one-item list, so a value that YAML parsed differently than meant
// is never silently accepted.
func validateExcludeType(raw any) error {
	switch list := raw.(type) {
	case nil:
		return nil
	case []string:
		for i, item := range list {
			if strings.TrimSpace(item) == "" {
				return fmt.Errorf("item %d is empty", i+1)
			}
		}
		return nil
	case []any:
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("item %d must be a resource type name (a string), got %T", i+1, item)
			}
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("item %d is empty", i+1)
			}
		}
		return nil
	default:
		return fmt.Errorf("must be a list of resource type names, got %T", raw)
	}
}

// SetDefaults registers the built-in defaults for settings that used to get one
// from a flag definition and now have no flag at all.
//
// "workers" deliberately gets NO default: its per-API counts (Microsoft Graph
// 5, ARM 20) apply unless the operator chose a single count deliberately, and
// that choice is detected with viper.IsSet. A default would make IsSet always
// true and silently flatten both API counts to one number.
func SetDefaults(v *viper.Viper) {
	v.SetDefault("timeout", DefaultTimeoutSeconds)
}

// resolveBaseFile picks the general configuration file: an explicit --config
// wins, otherwise the base.yaml a config directory holds by convention. An
// explicit path that cannot be read is an error, so a mistyped --config is
// never silently ignored; a config directory without a base.yaml is normal.
func resolveBaseFile(opts Options) (string, error) {
	if opts.ConfigFile != "" {
		if _, err := os.Stat(opts.ConfigFile); err != nil {
			return "", fmt.Errorf("failed to read config file %q: %w", opts.ConfigFile, err)
		}
		return opts.ConfigFile, nil
	}
	if opts.ConfigDir == "" {
		return "", nil
	}
	base := filepath.Join(opts.ConfigDir, BaseFileName)
	if info, err := os.Stat(base); err != nil || info.IsDir() {
		return "", nil
	}
	return base, nil
}

// resolveProfileFile picks the tenant profile inside the config directory. The
// domain must be a single path segment so a flag value can never escape the
// directory, and a missing profile is fatal with the available domains listed.
func resolveProfileFile(opts Options) (string, error) {
	if opts.ConfigDir == "" {
		return "", nil
	}
	if err := ValidateDomain(opts.Domain); err != nil {
		return "", err
	}

	path := filepath.Join(opts.ConfigDir, opts.Domain+profileExt)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		available := ProfileCandidates(opts.ConfigDir)
		if len(available) == 0 {
			return "", fmt.Errorf("%w: expected %s (the directory holds no profiles)", ErrProfileMissing, path)
		}
		return "", fmt.Errorf("%w: expected %s (available: %s)", ErrProfileMissing, path, strings.Join(available, ", "))
	}
	return path, nil
}

// ValidateDomain rejects a domain that is not a single path segment, so joining
// it to the config directory or the output directory can never escape either.
func ValidateDomain(domain string) error {
	switch {
	case domain == "":
		return errors.New("the tenant domain must not be empty")
	case domain == "." || domain == "..":
		return fmt.Errorf("invalid tenant domain %q", domain)
	case strings.ContainsAny(domain, `/\`):
		return fmt.Errorf("invalid tenant domain %q: it must be a single name, not a path", domain)
	case filepath.Base(domain) != domain:
		return fmt.Errorf("invalid tenant domain %q: it must be a single name, not a path", domain)
	case strings.EqualFold(domain, strings.TrimSuffix(BaseFileName, profileExt)):
		return fmt.Errorf("invalid tenant domain %q: that name is reserved for the base configuration (%s)", domain, BaseFileName)
	}
	return nil
}

// ProfileCandidates lists the tenant domains a config directory holds profiles
// for, sorted. base.yaml is excluded explicitly rather than incidentally. It
// never fails: an unreadable directory simply has no candidates, so this can be
// used both in an error message and in shell completion.
func ProfileCandidates(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var domains []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != profileExt {
			continue
		}
		if strings.EqualFold(entry.Name(), BaseFileName) {
			continue
		}
		domains = append(domains, strings.TrimSuffix(entry.Name(), profileExt))
	}
	sort.Strings(domains)
	return domains
}

// validateFile reads one configuration file's top-level keys and checks them
// against the partition. It parses the YAML directly rather than going through
// viper so the check sees exactly the keys this one file declares.
func validateFile(path string, want Scope) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}
	return ValidateKeys(path, keys, want)
}
