package cmd

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"

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

// TestConfigExampleIsNoOp guards the promise made in config.example.yaml's own
// header: loading the file unmodified must behave exactly like running with no
// config file at all. Every effective value it produces must equal the built-in
// default a plain `azure-rd download` uses, INCLUDING the transform-config hash
// recorded in resources/metadata.yaml — otherwise a run that loads the example
// would silently rewrite every resource's recorded hash. Any drift in the file
// (a re-defaulted key, a spelled-out transformer setting) breaks this test.
func TestConfigExampleIsNoOp(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevConfig := flagConfigFile
	t.Cleanup(func() { flagConfigFile = prevConfig })

	flagConfigFile = filepath.Join("..", "config.example.yaml")
	initConfig()

	// flagDefault returns the built-in default of a download/root flag as its
	// string form so this test never re-hardcodes a default that could drift
	// from the flag definition.
	flagDefault := func(name string) string {
		if f := downloadCmd.Flags().Lookup(name); f != nil {
			return f.DefValue
		}
		if f := rootCmd.PersistentFlags().Lookup(name); f != nil {
			return f.DefValue
		}
		t.Fatalf("no flag named %q on download or root command", name)
		return ""
	}

	// Scalar keys: the value loaded from config.example.yaml must equal the
	// flag's built-in default, so flag > env > config > default collapses to the
	// same value whether or not the file is loaded.
	for _, k := range []string{"subscription", "client-id", "tenant-id", "output", "resource-group", "log-level"} {
		if got, want := viper.GetString(k), flagDefault(k); got != want {
			t.Errorf("config.example.yaml %q = %q, want built-in default %q", k, got, want)
		}
	}

	for _, k := range []string{"dry-run", "resolve-secrets", "no-prompt", "prune"} {
		want := flagDefault(k) == "true"
		if got := viper.GetBool(k); got != want {
			t.Errorf("config.example.yaml %q = %v, want built-in default %v", k, got, want)
		}
	}

	for _, k := range []string{"workers", "timeout"} {
		want, err := strconv.Atoi(flagDefault(k))
		if err != nil {
			t.Fatalf("flag %q default %q is not an int: %v", k, flagDefault(k), err)
		}
		if got := viper.GetInt(k); got != want {
			t.Errorf("config.example.yaml %q = %d, want built-in default %d", k, got, want)
		}
	}

	for _, k := range []string{"resource-id", "type"} {
		if got := viper.GetStringSlice(k); len(got) != 0 {
			t.Errorf("config.example.yaml %q = %v, want empty (built-in default)", k, got)
		}
	}

	// Non-flag, config-only sections must resolve to the same values the
	// download command builds when no config file is present.
	if got, want := buildWorkerConfig(), models.DefaultWorkerConfig(); !reflect.DeepEqual(got, want) {
		t.Errorf("buildWorkerConfig() from config.example.yaml = %+v, want default %+v", got, want)
	}

	gotTransformers := buildTransformerConfigs()
	wantTransformers := models.DefaultTransformerConfigs()
	if !reflect.DeepEqual(gotTransformers, wantTransformers) {
		t.Errorf("buildTransformerConfigs() from config.example.yaml = %+v, want default %+v", gotTransformers, wantTransformers)
	}

	if got := buildResourceFilters(); len(got) != 0 {
		t.Errorf("buildResourceFilters() from config.example.yaml = %+v, want none (filters must stay commented out)", got)
	}

	// The taxonomy section changes docs/index.yaml when active, so it must stay
	// commented out in the example to preserve the no-op guarantee.
	if viper.IsSet("taxonomy") {
		t.Errorf("config.example.yaml sets 'taxonomy' = %v, want unset (taxonomy must stay commented out)", viper.Get("taxonomy"))
	}

	// The transform-config hash written to resources/metadata.yaml must be
	// byte-for-byte identical, or a run loading the example would report every
	// resource as changed even though the output bytes are unchanged.
	gotHash := docs.HashTransformConfig(gotTransformers, viper.GetBool("resolve-secrets"))
	wantHash := docs.HashTransformConfig(wantTransformers, false)
	if gotHash != wantHash {
		t.Errorf("transformConfigSha256 from config.example.yaml = %s, want default %s", gotHash, wantHash)
	}
}
