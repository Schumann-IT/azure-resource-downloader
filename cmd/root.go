package cmd

import (
	"fmt"
	"os"

	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Package-level flag variables are prefixed with "flag" so they never collide
// with (and shadow) local variables in command implementations, which commonly
// read the same settings back from Viper using natural names like dryRun or
// resourceGroup. These are referenced only here in root.go for flag binding;
// command code reads values via viper.Get*.
var (
	flagConfigFile    string
	flagSubscription  string
	flagOutput        string
	flagWorkers       int
	flagDryRun        bool
	flagLogLevel      string
	flagClientID      string
	flagTenantID      string
	flagResourceTypes []string
	flagResourceGroup string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "azure-rd",
	Short: "Azure Resource Downloader - Download and transform Azure resources",
	Long: `Azure Resource Downloader is a CLI tool that downloads Azure resources
and transforms them into clean YAML format.

The tool follows a pipeline pattern with async processing for maximum performance.
It's designed to be easily extensible with support for multiple Azure resource types.

Authentication reuses your existing Azure CLI session (run 'az login' first); the
same delegated token is used for both ARM and Microsoft Graph calls. To download
Microsoft Graph/Intune types that need scopes the Azure CLI app cannot provide,
sign in to a dedicated app registration with --client-id/--tenant-id (device-code flow).`,
	Version: "1.0.0",
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

	// Global flags
	rootCmd.PersistentFlags().StringVar(&flagConfigFile, "config", "", "path to a YAML config file; if omitted, no config file is loaded and defaults apply")
	rootCmd.PersistentFlags().StringVar(&flagSubscription, "subscription", "", "Azure subscription ID (default: your az login default subscription)")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "./output", "directory to write downloaded resources into")
	rootCmd.PersistentFlags().IntVar(&flagWorkers, "workers", 5, "number of concurrent workers")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "preview what would be downloaded without writing files")
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "info", "log verbosity: debug, info, warn, or error")
	rootCmd.PersistentFlags().StringVar(&flagClientID, "client-id", "", "app registration (client) ID for device-code sign-in; use to obtain Graph scopes the az login app lacks (e.g. DeviceManagementConfiguration.ReadWrite.All)")
	rootCmd.PersistentFlags().StringVar(&flagTenantID, "tenant-id", "", "Entra tenant ID for device-code sign-in (required with --client-id)")
	rootCmd.PersistentFlags().StringSliceVar(&flagResourceTypes, "type", []string{}, "resource type to download; repeatable, acts as a filter (default: all registered types)")
	rootCmd.PersistentFlags().StringVar(&flagResourceGroup, "resource-group", "", "download resources in this resource group")

	// Bind flags to viper
	_ = viper.BindPFlag("subscription", rootCmd.PersistentFlags().Lookup("subscription"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
	_ = viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("client-id", rootCmd.PersistentFlags().Lookup("client-id"))
	_ = viper.BindPFlag("tenant-id", rootCmd.PersistentFlags().Lookup("tenant-id"))
	_ = viper.BindPFlag("type", rootCmd.PersistentFlags().Lookup("type"))
	_ = viper.BindPFlag("resource-group", rootCmd.PersistentFlags().Lookup("resource-group"))
}

// initConfig reads environment variables and, only when --config is given, the
// specified configuration file. Without --config, no config file is loaded and
// the built-in defaults apply (still overridable by flags and AZURE_RD_* env
// vars). An explicitly requested config file that cannot be read is fatal.
func initConfig() {
	// Read in environment variables that match
	viper.SetEnvPrefix("AZURE_RD")
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
	// Priority: CLI flag > config file > env variable > default
	configuredLevel := viper.GetString("log-level")
	if configuredLevel != "" {
		logger.SetLogLevel(configuredLevel)
	}

	// Log config file usage after logger is configured
	if configFileUsed != "" {
		logger.Default.Info("Using config file", "path", configFileUsed)
	}
}
