package cmd

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// version is the tool version reported by `--version` and recorded in export
// metadata. It is empty in a plain `go build` and injected at release time via
// -ldflags "-X azure-resource-downloader/cmd.version=<v>". When unset,
// resolveVersion falls back to the module version or VCS revision the Go
// toolchain embeds, so the reported version tracks the binary rather than a
// literal that never moves.
var version = ""

// Package-level flag variables are prefixed with "flag" so they never collide
// with (and shadow) local variables in command implementations, which commonly
// read the same settings back from Viper using natural names like dryRun.
// These are referenced only here in root.go for flag binding; command code
// reads values via viper.Get*.
var (
	flagConfigFile string
	flagOutput     string
	flagDryRun     bool
	flagLogLevel   string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "azure-rd",
	Short: "Azure Resource Downloader - Download and transform Azure resources",
	Long: `Azure Resource Downloader is a CLI tool that downloads Azure resources,
transforms them into clean YAML format, and generates per-resource-type AI
documentation prompts by default (pass --no-prompt to skip them).

The tool follows a pipeline pattern with async processing for maximum performance.
It's designed to be easily extensible with support for multiple Azure resource types.

Authentication reuses your existing Azure CLI session (run 'az login' first); the
same delegated token is used for both ARM and Microsoft Graph calls. To download
Microsoft Graph/Intune types that need scopes the Azure CLI app cannot provide,
sign in to a dedicated app registration with --client-id/--tenant-id (device-code flow).`,
	Version: resolveVersion(),
}

// resolveVersion returns the tool version, preferring an ldflags-injected value,
// then the module version of a `go install …@version` build, then the embedded
// VCS revision (with a -dirty suffix for uncommitted changes), and finally
// "dev". This is why toolVersion in metadata.yaml actually moves when the binary
// does — letting a later docs step attribute a prompt-hash change to an upgrade
// rather than reporting it as content drift.
func resolveVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if dirty {
			rev += "-dirty"
		}
		return rev
	}
	return "dev"
}

// toolVersion returns the tool identifier recorded in export metadata, derived
// from the root command's version so there is a single source of truth.
func toolVersion() string {
	return "azure-rd " + rootCmd.Version
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags: every planned command needs the export root and benefits
	// from a uniform dry-run safety switch, config loading and log verbosity.
	// Command-specific flags (auth, selection, pipeline tuning) are opt-in via
	// the helpers in internal/cmdutil so future commands do not silently inherit them.
	rootCmd.PersistentFlags().StringVar(&flagConfigFile, "config", "", "path to a YAML config file; if omitted, no config file is loaded and defaults apply")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "./output", "directory to write downloaded resources into")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "preview what would be downloaded without writing files")
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "info", "log verbosity: debug, info, warn, or error")

	// Bind global flags to viper. Command-local flags are bound per-execution in
	// each command's RunE via cmdutil.BindFlags to avoid the global viper
	// singleton picking up a sibling command's identically named flag.
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))

	// Subcommand packages that live in their own directory register through a
	// constructor (they cannot import package cmd without a cycle). Flat-file
	// commands in package cmd still self-register in their own init().
	rootCmd.AddCommand(NewCommand())
}

// initConfig reads environment variables and, only when --config is given, the
// specified configuration file. Without --config, no config file is loaded and
// the built-in defaults apply (still overridable by flags and AZURE_RD_* env
// vars). An explicitly requested config file that cannot be read is fatal.
func initConfig() {
	// Read in environment variables that match. The key replacer maps hyphens
	// to underscores so hyphenated keys like log-level resolve to a shell-
	// exportable name (AZURE_RD_LOG_LEVEL) rather than AZURE_RD_LOG-LEVEL.
	viper.SetEnvPrefix("AZURE_RD")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	configFileUsed := ""
	if flagConfigFile != "" {
		// A config file was explicitly requested; load it or fail loudly so a
		// mistyped path is never silently ignored.
		viper.SetConfigFile(flagConfigFile)
		if err := viper.ReadInConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read config file %q: %v\n", flagConfigFile, err)
			os.Exit(1)
		}
		configFileUsed = viper.ConfigFileUsed()
	}

	// Configure log level after reading config
	// Priority: CLI flag > env variable > config file > default
	configuredLevel := viper.GetString("log-level")
	if configuredLevel != "" {
		logger.SetLogLevel(configuredLevel)
	}

	// Log config file usage after logger is configured
	if configFileUsed != "" {
		logger.Default.Info("Using config file", "path", configFileUsed)
	}
}
