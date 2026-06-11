package cmd

import (
	"fmt"
	"os"

	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile        string
	subscriptionID string
	outputDir      string
	workerCount    int
	dryRun         bool
	logLevel       string
	resolveSecrets bool
	clientID       string
	tenantID       string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "azure-rd",
	Short: "Azure Resource Downloader - Download and transform Azure resources",
	Long: `Azure Resource Downloader is a CLI tool that downloads Azure resources,
transforms them into clean YAML format, and generates Terraform import statements.

The tool follows a pipeline pattern with async processing for maximum performance.
It's designed to be easily extensible with support for multiple Azure resource types.`,
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.azure-rd.yaml)")
	rootCmd.PersistentFlags().StringVar(&subscriptionID, "subscription", "", "Azure subscription ID (optional, uses default from az login if not specified)")
	rootCmd.PersistentFlags().StringVar(&outputDir, "output", "./output", "output directory for downloaded resources")
	rootCmd.PersistentFlags().IntVar(&workerCount, "workers", 5, "number of concurrent workers")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "dry run mode (don't write files)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&resolveSecrets, "resolve-secrets", false, "resolve masked (encrypted) Intune OMA-URI secret values to plaintext (writes secrets to output)")
	rootCmd.PersistentFlags().StringVar(&clientID, "client-id", "", "app registration (client) ID for device-code sign-in (optional; defaults to the az login session). Enables Graph scopes the Azure CLI app cannot obtain, e.g. DeviceManagementConfiguration.ReadWrite.All")
	rootCmd.PersistentFlags().StringVar(&tenantID, "tenant-id", "", "Entra tenant ID for device-code sign-in (used with --client-id)")

	// Bind flags to viper
	_ = viper.BindPFlag("subscription", rootCmd.PersistentFlags().Lookup("subscription"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
	_ = viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("resolve-secrets", rootCmd.PersistentFlags().Lookup("resolve-secrets"))
	_ = viper.BindPFlag("client-id", rootCmd.PersistentFlags().Lookup("client-id"))
	_ = viper.BindPFlag("tenant-id", rootCmd.PersistentFlags().Lookup("tenant-id"))
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		// Search config in home directory with name ".azure-rd" (without extension)
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".azure-rd")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("AZURE_RD")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	configFileUsed := ""
	if err := viper.ReadInConfig(); err == nil {
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
