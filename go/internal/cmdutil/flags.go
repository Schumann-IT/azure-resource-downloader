// Package cmdutil holds CLI helpers shared by the root command package and its
// subcommand packages (e.g. cmd/docs). It exists so a subcommand living in its
// own directory can reuse the flag-group and viper-binding helpers without
// importing package cmd, which would create an import cycle (package cmd must
// import the subcommand packages to register them).
package cmdutil

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Built-in defaults shared by the flag definitions so a single change here can
// never silently invert value-sniffing logic elsewhere.
const (
	DefaultWorkerCount    = 5
	DefaultTimeoutSeconds = 300
)

// AddAzureAuthFlags registers the Azure authentication flags locally on cmd:
// --subscription, --client-id and --tenant-id. client-id and tenant-id are
// marked required-together to enforce the device-code contract their help text
// documents.
func AddAzureAuthFlags(cmd *cobra.Command) {
	defineAzureAuthFlags(cmd.Flags())
	cmd.MarkFlagsRequiredTogether("client-id", "tenant-id")
}

// AddPersistentAzureAuthFlags registers the same authentication flags on cmd's
// persistent flag set, so every subcommand of a command group inherits one
// definition instead of declaring its own copy.
//
// The required-together pairing is deliberately NOT declared here: Cobra
// validates flag groups against a command's own flag set, so marking it on the
// parent would not constrain the subcommand that actually runs. Enforce it with
// RequireAuthFlagPair in the group's persistent pre-run instead.
func AddPersistentAzureAuthFlags(cmd *cobra.Command) {
	defineAzureAuthFlags(cmd.PersistentFlags())
}

// defineAzureAuthFlags is the single definition of the authentication flags'
// names, defaults and usage strings, so a local and a persistent registration
// can never drift into two spellings of the same flag.
func defineAzureAuthFlags(f *pflag.FlagSet) {
	f.String("subscription", "", "Azure subscription ID (default: your az login default subscription)")
	f.String("client-id", "", "app registration (client) ID for device-code sign-in; use to obtain Graph scopes the az login app lacks (e.g. DeviceManagementConfiguration.ReadWrite.All)")
	f.String("tenant-id", "", "Entra tenant ID for device-code sign-in (required with --client-id)")
}

// RequireAuthFlagPair reports an error when exactly one of --client-id and
// --tenant-id is set on cmd, reproducing MarkFlagsRequiredTogether for flags a
// command inherits from its parent rather than declares itself.
func RequireAuthFlagPair(cmd *cobra.Command) error {
	clientID := cmd.Flags().Lookup("client-id")
	tenantID := cmd.Flags().Lookup("tenant-id")
	if clientID == nil || tenantID == nil {
		return nil
	}
	if clientID.Changed != tenantID.Changed {
		return errors.New("if any flags in the group [client-id tenant-id] are set they must all be set; missing " +
			missingAuthFlagName(clientID.Changed))
	}
	return nil
}

// missingAuthFlagName names the half of the client-id/tenant-id pair that was
// left unset, given whether client-id was the one provided.
func missingAuthFlagName(clientIDSet bool) string {
	if clientIDSet {
		return "[tenant-id]"
	}
	return "[client-id]"
}

// AddSelectionFlags registers the download selection triad locally on cmd:
// --resource-id, --type and --resource-group. All three choose what to
// download and are declared identically so each can also be set via config or
// an AZURE_RD_* env var.
func AddSelectionFlags(cmd *cobra.Command) {
	defineSelectionFlags(cmd.Flags())
}

// AddPersistentSelectionFlags registers the selection triad on cmd's persistent
// flag set, for a command group whose every subcommand honours it.
func AddPersistentSelectionFlags(cmd *cobra.Command) {
	defineSelectionFlags(cmd.PersistentFlags())
}

// defineSelectionFlags is the single definition of the selection triad.
func defineSelectionFlags(f *pflag.FlagSet) {
	f.StringSlice("resource-id", []string{}, "explicit Azure resource ID to download; repeatable")
	f.StringSlice("type", []string{}, "resource type to download; repeatable, acts as a filter (default: all registered types)")
	f.String("resource-group", "", "download resources in this resource group")
}

// AddPipelineFlags registers the pipeline tuning flags locally on cmd:
// --workers and --timeout.
func AddPipelineFlags(cmd *cobra.Command) {
	definePipelineFlags(cmd.Flags())
}

// AddPersistentPipelineFlags registers the pipeline tuning flags on cmd's
// persistent flag set, for a command group whose every subcommand honours them.
func AddPersistentPipelineFlags(cmd *cobra.Command) {
	definePipelineFlags(cmd.PersistentFlags())
}

// definePipelineFlags is the single definition of the pipeline tuning flags.
func definePipelineFlags(f *pflag.FlagSet) {
	f.Int("workers", DefaultWorkerCount, "number of concurrent workers")
	f.Int("timeout", DefaultTimeoutSeconds, "per-operation timeout in seconds (applied around each resource fetch)")
}

// BindFlags binds every flag that applies to cmd to viper so each value can
// also be supplied via the config file or an AZURE_RD_* environment variable
// (precedence: flag > env > config > default). Binding happens per-execution to
// avoid the global viper singleton picking up a sibling command's identically
// named flag.
//
// It binds inherited flags as well as locally declared ones: a flag a command
// group declares once on its parent still has to resolve through env and config
// for the subcommand that reads it. Binding only local flags would leave such a
// flag working on the command line while silently ignoring AZURE_RD_* and the
// config file — a failure invisible to anything that only tests flags.
func BindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = viper.BindPFlag(f.Name, f)
	})
}
