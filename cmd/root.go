package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile        string
	subscriptionID string
	outputDir      string
	workerCount    int
	dryRun         bool
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
	rootCmd.PersistentFlags().StringVar(&subscriptionID, "subscription", "", "Azure subscription ID (required)")
	rootCmd.PersistentFlags().StringVar(&outputDir, "output", "./output", "output directory for downloaded resources")
	rootCmd.PersistentFlags().IntVar(&workerCount, "workers", 5, "number of concurrent workers")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "dry run mode (don't write files)")

	// Bind flags to viper
	viper.BindPFlag("subscription", rootCmd.PersistentFlags().Lookup("subscription"))
	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
	viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))

	// Mark required flags
	rootCmd.MarkPersistentFlagRequired("subscription")
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
		viper.SetConfigType("yaml")
		viper.SetConfigName(".azure-rd")
	}

	// Read in environment variables that match
	viper.SetEnvPrefix("AZURE_RD")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
