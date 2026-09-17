package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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
	flagDebug      bool
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "azure-rd",
	Short: "Export and document an Entra ID / Intune tenant's configuration",
	Long: `azure-rd exports the configuration of an Entra ID / Intune tenant (plus a
few Azure Resource Manager types) as clean, reproducible YAML, and drives the
incremental, AI-generated documentation of that export. Per-resource-type AI
documentation prompts are written by default (pass --no-prompt to skip them).

The tool follows a pipeline pattern with async processing for maximum performance.
It's designed to be easily extensible with support for multiple Azure resource types.

Authentication reuses your existing Azure CLI session (run 'az login' first); the
same delegated token is used for both ARM and Microsoft Graph calls. To download
Microsoft Graph/Intune types that need scopes the Azure CLI app cannot provide,
sign in to a dedicated app registration with --client-id/--tenant-id (device-code flow).

Run 'azure-rd --debug' (with no subcommand) to print a diagnostic report of the
current Azure session — how the tool is authenticated, who is signed in, and which
tenant, subscription and output directory a download would use right now. It writes
nothing and is safe to run at any time.`,
	Version: version.Resolve(),
	RunE:    runRoot,
	// Errors are printed exactly once, in execute(); without this Cobra prints a
	// returned error and execute() would print it again.
	SilenceErrors: true,
	// Config loading lives here rather than in cobra.OnInitialize because an
	// initializer cannot return an error, and a mistyped --config path must fail
	// loudly instead of calling os.Exit inline. EnableTraverseRunHooks (set in
	// init) makes this run even when a command group declares its own hook.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Flags parsed fine, so any later error is a runtime failure for which
		// usage output would be noise. Flag and unknown-command errors happen
		// before this hook and still print usage.
		cmd.SilenceUsage = true
		return initConfig()
	},
}

// Execute runs the root command and translates a returned error into the
// process exit code. It is the only place in the program that calls os.Exit.
func Execute() {
	if code := execute(); code != 0 {
		os.Exit(code)
	}
}

// execute runs the root command under an interrupt-cancellable context and
// returns the process exit code. It is the single error print site: Cobra's own
// printing is silenced on root, and commands return errors instead of printing
// and exiting inline.
func execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal has cancelled the context, restore the default
	// signal behaviour so a second Ctrl+C force-quits a run stuck in a write.
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		rootCmd.PrintErrln(rootCmd.ErrPrefix(), err.Error())
		return cmdutil.ExitCode(err)
	}
	return 0
}

func init() {
	// Run every parent's PersistentPreRunE down the chain, not only the nearest
	// one: root's hook loads the config for all commands, while a group like
	// `resource` keeps its own hook for its flag-pair validation.
	cobra.EnableTraverseRunHooks = true

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

	// The root command itself does one thing besides dispatching to subcommands:
	// with --debug (and no subcommand) it prints a diagnostic report of the
	// current Azure session. That report authenticates exactly like download, so
	// the auth flags are registered locally on root — NOT persistently, so other
	// top-level commands do not silently inherit them.
	//
	// This is deliberately a second registration of the same group: the resource
	// command group declares it persistently for its own subcommands (see
	// newResourceCommand). Both are needed — root's copy serves --debug, the
	// group's copy serves its subcommands — and neither is redundant.
	rootCmd.Flags().BoolVar(&flagDebug, "debug", false, "print a diagnostic report of the current Azure session and exit (no files written)")
	cmdutil.AddAzureAuthFlags(rootCmd)

	// Subcommand packages that live in their own directory register through a
	// constructor (they cannot import package cmd without a cycle).
	rootCmd.AddCommand(newResourceCommand())
	rootCmd.AddCommand(NewCommand())
}

// initConfig reads environment variables and, only when --config is given, the
// specified configuration file. Without --config, no config file is loaded and
// the built-in defaults apply (still overridable by flags and AZURE_RD_* env
// vars). An explicitly requested config file that cannot be read is an error,
// so a mistyped --config path is never silently ignored.
func initConfig() error {
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
			return fmt.Errorf("failed to read config file %q: %w", flagConfigFile, err)
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
	return nil
}

// runRoot handles a bare `azure-rd` invocation (no subcommand). With --debug it
// prints the current Azure session report; otherwise it prints help, matching
// Cobra's default behaviour for a command with no action.
func runRoot(cmd *cobra.Command, args []string) error {
	if !flagDebug {
		return cmd.Help()
	}
	return runDebugReport(cmd)
}

// runDebugReport authenticates exactly as `download` would and reports how the
// tool is authenticated, who is signed in, and which tenant, subscription and
// output directory a download would use right now. It writes nothing.
func runDebugReport(cmd *cobra.Command) error {
	// Bind this command's local flags to viper before reading any values so the
	// flag > env > config > default precedence holds.
	cmdutil.BindFlags(cmd)

	ctx := cmd.Context()
	log := logger.Default

	sub := viper.GetString("subscription")
	clientID := viper.GetString("client-id")
	tenantID := viper.GetString("tenant-id")
	output := viper.GetString("output")

	// Report the static session facts before any network call so they are shown
	// even if authentication later fails.
	log.Info("azure-rd", "version", cmd.Root().Version)
	if clientID != "" {
		log.Info("Authentication", "method", "device-code sign-in (dedicated app registration)",
			"client_id", clientID, "tenant_id", tenantID)
	} else {
		log.Info("Authentication", "method", "Azure CLI session (az login)")
	}
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		log.Info("Config file", "path", configFile)
	} else {
		log.Info("Config file", "path", "<none>")
	}

	// Authenticate exactly as download does (auto-detecting a subscription when
	// none is given; a missing subscription is not fatal for Graph-only access).
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub, clientID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	// Resolve the signed-in principal from the access token claims (best-effort:
	// no directory call, so a token that is opaque or lacks the claims warns
	// rather than fails).
	if id, err := azure.SignedInIdentity(ctx, azureClient.GetCredential()); err != nil {
		log.Warn("Could not resolve the signed-in identity", "reason", azure.ErrorSummary(err))
		log.Debug("Identity resolution failed", "error", err)
	} else {
		log.Info("Signed in", "user", id.Username, "tenant_id", id.TenantID, "object_id", id.ObjectID)
	}

	// Subscription: the value actually resolved (may have been auto-detected).
	if sub = azureClient.GetSubscriptionID(); sub == "" {
		log.Info("Subscription", "id", "<none> (Microsoft Graph resources only; ARM types are skipped)")
	} else {
		log.Info("Subscription", "id", sub)
	}

	// Tenant default domain (best-effort). When resolved, a download scopes its
	// output under it, so report the same effective path here.
	if domain, err := azureClient.GetTenantDomain(ctx); err != nil {
		log.Warn("Could not resolve the tenant domain", "reason", azure.ErrorSummary(err))
		log.Debug("Tenant domain resolution failed", "error", err)
	} else {
		log.Info("Tenant", "default_domain", domain)
		output = filepath.Join(output, domain)
	}
	log.Info("Output directory", "path", output)

	// How many resource types this session could download. Enumerating them is a
	// local operation; secret resolution is a download-only concern, so off here.
	registry := handlers.NewRegistry(azureClient.GetCredential(), sub, false)
	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	return nil
}
