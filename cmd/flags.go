package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Built-in defaults shared by the flag definitions so a single change here can
// never silently invert value-sniffing logic elsewhere.
const (
	defaultWorkerCount    = 5
	defaultTimeoutSeconds = 300
)

// addAzureAuthFlags registers the Azure authentication flags on cmd:
// --subscription, --client-id and --tenant-id. client-id and tenant-id are
// marked required-together to enforce the device-code contract their help text
// documents.
func addAzureAuthFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("subscription", "", "Azure subscription ID (default: your az login default subscription)")
	f.String("client-id", "", "app registration (client) ID for device-code sign-in; use to obtain Graph scopes the az login app lacks (e.g. DeviceManagementConfiguration.ReadWrite.All)")
	f.String("tenant-id", "", "Entra tenant ID for device-code sign-in (required with --client-id)")
	cmd.MarkFlagsRequiredTogether("client-id", "tenant-id")
}

// addSelectionFlags registers the download selection triad on cmd:
// --resource-id, --type and --resource-group. All three choose what to
// download and are declared identically so each can also be set via config or
// an AZURE_RD_* env var.
func addSelectionFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringSlice("resource-id", []string{}, "explicit Azure resource ID to download; repeatable")
	f.StringSlice("type", []string{}, "resource type to download; repeatable, acts as a filter (default: all registered types)")
	f.String("resource-group", "", "download resources in this resource group")
}

// addPipelineFlags registers the pipeline tuning flags on cmd: --workers and
// --timeout.
func addPipelineFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.Int("workers", defaultWorkerCount, "number of concurrent workers")
	f.Int("timeout", defaultTimeoutSeconds, "per-operation timeout in seconds (applied around each resource fetch)")
}

// bindFlags binds every local flag of cmd to viper so each value can also be
// supplied via the config file or an AZURE_RD_* environment variable
// (precedence: flag > env > config > default). Binding happens per-execution to
// avoid the global viper singleton picking up a sibling command's identically
// named flag.
func bindFlags(cmd *cobra.Command) {
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		_ = viper.BindPFlag(f.Name, f)
	})
}
