package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/tenantdir"
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
	flagConfigDir  string
	flagOutput     string
	flagDryRun     bool
	flagLogLevel   string
	flagDebug      bool
	flagDomain     string
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
		return initConfig(cmd)
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
	rootCmd.PersistentFlags().StringVar(&flagConfigFile, "config", "", "path to the base YAML config file; if omitted, no base file is loaded and defaults apply")
	rootCmd.PersistentFlags().StringVar(&flagConfigDir, "config-dir", "", "directory of per-tenant profiles (<domain>.yaml) plus an optional "+config.BaseFileName+"; requires --domain")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "./output", "directory to write downloaded resources into")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "preview what would be downloaded without writing files")
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "info", "log verbosity: debug, info, warn, or error")

	// Bind the global flags that commands read back through viper. Every flag
	// bound here is declared exactly once, which is what makes a single global
	// binding safe: there is no sibling command with a same-named flag to steal
	// it, so no per-execution re-binding is needed. --output is config-backed
	// (the file may set it, the flag overrides); --dry-run and --log-level are
	// flag-only and are bound purely so they can be read uniformly — a config
	// file that sets either is rejected by the key partition.
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))

	// The root command itself does one thing besides dispatching to subcommands:
	// with --debug (and no subcommand) it prints a diagnostic report of the
	// current Azure session. --domain is registered locally alongside it so the
	// report can select a tenant profile and show the configuration a real run
	// would use; it is NOT persistent, so other top-level commands do not
	// silently inherit it.
	rootCmd.Flags().BoolVar(&flagDebug, "debug", false, "print a diagnostic report of the current Azure session and exit (no files written)")
	rootCmd.Flags().StringVar(&flagDomain, "domain", "", "tenant domain whose profile to report on (selects <config-dir>/<domain>.yaml)")
	cmdutil.RegisterDomainCompletion(rootCmd)

	// Subcommand packages that live in their own directory register through a
	// constructor (they cannot import package cmd without a cycle).
	rootCmd.AddCommand(newResourceCommand())
	rootCmd.AddCommand(NewCommand())
}

// initConfig loads the configuration for the command about to run: the base
// file (--config, or the base.yaml a --config-dir holds by convention) and,
// when a tenant domain was explicitly passed, that tenant's profile merged over
// it. There is no environment layer: configuration reaches the tool through
// these files and the few command-line flags only, so a value can never arrive
// from a variable left over from another tenant's run.
//
// It takes the command being executed because the profile is selected by that
// command's explicitly passed --domain. Only an explicit flag counts: for a
// download the tenant is not known until after authentication, and the profile
// is what supplies the credentials authentication needs.
func initConfig(cmd *cobra.Command) error {
	res, err := config.Load(viper.GetViper(), config.Options{
		ConfigFile: flagConfigFile,
		ConfigDir:  flagConfigDir,
		Domain:     cmdutil.DeclaredDomain(cmd),
	})
	if err != nil {
		return err
	}
	configLoaded = res

	// The log level is a flag-only setting, so it is known before any file is
	// read; applying it after the load keeps the ordering harmless either way.
	if flagLogLevel != "" {
		logger.SetLogLevel(flagLogLevel)
	}

	switch {
	case res.ProfileFile != "" && res.BaseFile != "":
		logger.Default.Info("Using configuration", "base", res.BaseFile, "profile", res.ProfileFile, "domain", res.Domain)
	case res.ProfileFile != "":
		logger.Default.Info("Using configuration", "profile", res.ProfileFile, "domain", res.Domain)
	case res.BaseFile != "":
		logger.Default.Info("Using configuration", "base", res.BaseFile)
	}
	return nil
}

// configLoaded records which configuration files the current execution used, so
// the --debug report can show them without resolving anything a second time.
var configLoaded config.Result

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

	// Which configuration this run resolved: the whole point of profiles is that
	// a switch is visible, so --debug must be able to confirm one without
	// running anything that writes.
	log.Info("Configuration",
		"config_dir", orNone(flagConfigDir),
		"base", orNone(configLoaded.BaseFile),
		"profile", orNone(configLoaded.ProfileFile),
		"domain", orNone(configLoaded.Domain))

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
	// output under it, so report the same effective path here — through the same
	// resolver a real run uses, so the reported path cannot differ from the one
	// that would be written, including a refusal when the two disagree.
	resolvedDomain, err := azureClient.GetTenantDomain(ctx)
	if err != nil {
		log.Warn("Could not resolve the tenant domain", "reason", azure.ErrorSummary(err))
		log.Debug("Tenant domain resolution failed", "error", err)
	} else {
		log.Info("Tenant", "default_domain", resolvedDomain)
	}
	if target, terr := tenantdir.Resolve(output, flagDomain, resolvedDomain); terr != nil {
		log.Warn("A run would refuse to act on this tenant", "reason", terr.Error())
	} else {
		if target.Unverified() {
			log.Warn("The tenant domain is taken from --domain and was NOT confirmed by a session")
		}
		log.Info("Output directory", "path", target.Dir)
	}

	// How many resource types this session could download. Enumerating them is a
	// local operation; secret resolution is a download-only concern, so off here.
	registry := handlers.NewRegistry(azureClient.GetCredential(), sub, false)
	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	return nil
}

// orNone renders an unset path or domain as "<none>", so the debug report never
// prints an empty value that could be read as a blank rather than absent.
func orNone(value string) string {
	if value == "" {
		return "<none>"
	}
	return value
}
