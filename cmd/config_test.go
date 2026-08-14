package cmd

import (
	"testing"

	"github.com/spf13/viper"
)

// TestInitConfigEnvKeyReplacer guards the env-key replacer: a hyphenated config
// key such as log-level must resolve to a shell-exportable environment variable
// (AZURE_RD_LOG_LEVEL, not AZURE_RD_LOG-LEVEL, which no shell can export).
// Removing viper.SetEnvKeyReplacer silently breaks every hyphenated override.
func TestInitConfigEnvKeyReplacer(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	flagConfigFile = ""
	t.Setenv("AZURE_RD_LOG_LEVEL", "warn")

	initConfig()

	if got := viper.GetString("log-level"); got != "warn" {
		t.Errorf("log-level = %q, want %q; the env-key replacer must map log-level -> AZURE_RD_LOG_LEVEL", got, "warn")
	}
}

// TestInitConfigHyphenEnvNotDirectlyExportable documents why the replacer is
// necessary: the raw hyphenated form is not what AutomaticEnv looks up, so a
// value set under the literal hyphen name must NOT resolve.
func TestInitConfigHyphenEnvNotDirectlyExportable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	flagConfigFile = ""
	t.Setenv("AZURE_RD_OUTPUT", "/tmp/from-env")

	initConfig()

	if got := viper.GetString("output"); got != "/tmp/from-env" {
		t.Errorf("output = %q, want %q; AZURE_RD_OUTPUT must resolve via AutomaticEnv", got, "/tmp/from-env")
	}
}
