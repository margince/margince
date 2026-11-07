// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

var (
	level = config.Define(config.Spec[string]{
		Key: "log.level", Env: "MARGINCE_LOG_LEVEL", Flag: "log-level",
		Type: config.Enum[string]("debug", "info", "warn"), Default: config.Some("info"), Doc: "d",
	})
	dsn = config.Define(config.Spec[string]{
		Key: "dsn", Env: "MARGINCE_DSN", Flag: "dsn",
		Type: config.NonEmpty, Secret: true, Doc: "d",
	})
)

var errLen = errors.New("must be at least 32 bytes")

type fixture struct {
	args  []string
	env   []string
	files map[string]string // name → body, written into one temp dir; "{dir}" in args/env is replaced
}

func load(t *testing.T, f fixture, settings ...config.Any) (*config.Config, error) {
	t.Helper()
	reg, src := sources(t, f, settings...)
	return config.Load(reg, src)
}

func sources(t *testing.T, f fixture, settings ...config.Any) (*config.Registry, config.Sources) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range f.files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sub := func(xs []string) []string {
		out := make([]string, len(xs))
		for i, x := range xs {
			out[i] = strings.ReplaceAll(x, "{dir}", dir)
		}
		return out
	}
	reg, err := config.NewRegistry(settings)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--config", filepath.Join(dir, "margince.yaml")}, sub(f.args)...)
	return reg, config.Sources{Args: args, Environ: sub(f.env)}
}

func TestEachSourceBeatsTheOnesBelowIt(t *testing.T) {
	files := map[string]string{
		"margince.yaml": "log: {level: warn}\n",
		"over.yaml":     "log: {level: info}\n",
		"x.env":         "MARGINCE_LOG_LEVEL=warn\n",
	}
	steps := []struct {
		f      fixture
		want   string
		source config.Source
	}{
		{fixture{files: map[string]string{}}, "info", config.SourceDefault},
		{fixture{files: files}, "warn", config.SourceBaseFile},
		{fixture{files: files, args: []string{"--config-file", "{dir}/over.yaml"}}, "info", config.SourceConfigFile},
		{fixture{files: files, args: []string{"--config-file", "{dir}/over.yaml", "--env-file", "{dir}/x.env"}}, "warn", config.SourceEnvFile},
		{fixture{files: files, args: []string{"--env-file", "{dir}/x.env"}, env: []string{"MARGINCE_LOG_LEVEL=debug"}}, "debug", config.SourceEnv},
		{fixture{files: files, args: []string{"--log-level=info"}, env: []string{"MARGINCE_LOG_LEVEL=debug"}}, "info", config.SourceFlag},
	}
	for i, s := range steps {
		cfg, err := load(t, s.f, level)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		v, origin := config.ValWithSource(cfg, level)
		if v != s.want || origin.Source != s.source {
			t.Errorf("step %d: got %q from %s, want %q from %s", i, v, origin.Source, s.want, s.source)
		}
	}
}

func TestEveryFaultIsReportedInOneBoot(t *testing.T) {
	_, err := load(t, fixture{
		args:  []string{"--log-level=loud"},
		files: map[string]string{"margince.yaml": "log: {lvl: x}\n", "margince.dev.yaml": ""},
	}, level, dsn)
	if err == nil {
		t.Fatal("want faults")
	}
	for _, want := range []string{
		"--log-level (flag): want one of debug, info, warn",
		"MARGINCE_DSN: required",
		"unknown key log.lvl",
		"margince.dev.yaml: overlay files are no longer read; pass it with --config-file",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestASecretNeverAppearsInAFault(t *testing.T) {
	short := config.Define(config.Spec[string]{
		Key: "key", Env: "X_KEY", Type: config.NonEmpty, Secret: true,
		Validate: func(s string) error {
			if len(s) < 32 {
				return errLen
			}
			return nil
		}, Doc: "d",
	})
	_, err := load(t, fixture{env: []string{"X_KEY=hunter2"}}, short)
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("got %v", err)
	}
}

func TestLocationsAndPostureComeOnlyFromFlagsOrEnv(t *testing.T) {
	_, err := load(t, fixture{
		args:  []string{"--env-file", "{dir}/x.env"},
		files: map[string]string{"x.env": "MARGINCE_ENV=prod\nMARGINCE_CONFIG_FILE=y.yaml\n"},
	}, level)
	if err == nil || !strings.Contains(err.Error(), "MARGINCE_ENV is read only from the environment") ||
		!strings.Contains(err.Error(), "MARGINCE_CONFIG_FILE is read only from a flag or the environment") {
		t.Fatalf("got %v", err)
	}
}

// An env file copied from the old template carries bare MARGINCE_ENV= and
// MARGINCE_CONFIG= lines; empty is unset, so they name nothing to refuse.
func TestAnEmptyLoaderNameInTheEnvFileIsUnset(t *testing.T) {
	_, err := load(t, fixture{
		args:  []string{"--env-file", "{dir}/x.env"},
		files: map[string]string{"x.env": "MARGINCE_ENV=\nMARGINCE_CONFIG=\nMARGINCE_LOG_LEVEL=debug\n"},
	}, level)
	if err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestANamedConfigFileThatIsMissingIsAFault(t *testing.T) {
	_, err := load(t, fixture{args: []string{"--config-file", "{dir}/absent.yaml"}}, level)
	if err == nil || !strings.Contains(err.Error(), "absent.yaml") {
		t.Fatalf("got %v", err)
	}
}

func TestAnUnknownNamespacedVariableOnlyWarns(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{ns + "LOG_LVL=x", ns + "TEST_DSN=x", "HOME=/"}}, level)
	if err != nil {
		t.Fatal(err)
	}
	if w := cfg.Warnings(); len(w) != 1 || w[0] != ns+"LOG_LVL (did you mean "+ns+"LOG_LEVEL?)" {
		t.Fatalf("warnings %v", w)
	}
}

func TestAVariableAnotherProgramReadsDoesNotWarn(t *testing.T) {
	reg, src := sources(t, fixture{env: []string{ns + "OTHER=x"}}, level)
	src.Known = config.Names{Envs: []string{ns + "OTHER"}}
	cfg, err := config.Load(reg, src)
	if err != nil {
		t.Fatal(err)
	}
	if w := cfg.Warnings(); len(w) != 0 {
		t.Fatalf("warnings %v", w)
	}
}

func TestTheOldOverlayLoadsWhenPassedWithConfigFile(t *testing.T) {
	cfg, err := load(t, fixture{
		args: []string{"--config-file", "{dir}/margince.dev.yaml"},
		files: map[string]string{
			"margince.yaml":     "log: {level: warn}\n",
			"margince.dev.yaml": "log: {level: debug}\n",
		},
	}, level)
	if err != nil {
		t.Fatal(err)
	}
	if v, origin := config.ValWithSource(cfg, level); v != "debug" || origin.Source != config.SourceConfigFile {
		t.Fatalf("got %q from %s", v, origin.Source)
	}
}

func TestAnExplicitLoaderFlagBeatsTheEnvironment(t *testing.T) {
	cfg, err := load(t, fixture{
		args:  []string{"--config-trace=false", "--config-file="},
		env:   []string{"MARGINCE_CONFIG_TRACE=true", "MARGINCE_CONFIG_FILE={dir}/over.yaml"},
		files: map[string]string{"over.yaml": "log: {level: debug}\n"},
	}, level)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Files().ConfigFile != "" || config.Val(cfg, level) != "info" {
		t.Fatalf("config file %q, level %q", cfg.Files().ConfigFile, config.Val(cfg, level))
	}
	if cfg.Trace() {
		t.Fatal("--config-trace=false lost to the environment")
	}
}

func TestASecretValidatorThatEchoesItsInputIsRedacted(t *testing.T) {
	echo := config.Define(config.Spec[string]{
		Key: "key", Env: "X_KEY", Type: config.NonEmpty, Secret: true,
		Validate: func(s string) error { return fmt.Errorf("bad %q", s) }, Doc: "d",
	})
	_, err := load(t, fixture{env: []string{"X_KEY=hunter2-secret"}}, echo)
	if err == nil || strings.Contains(err.Error(), "hunter2") || !strings.Contains(err.Error(), "X_KEY (env)") {
		t.Fatalf("got %v", err)
	}
}

// A secret may carry no default, so a type that takes empty text is what makes
// it optional; NonEmpty keeps it required.
func TestUnsetIsAFaultOnlyForATypeWithNoEmptyForm(t *testing.T) {
	optional := config.Define(config.Spec[string]{Key: "metrics_token", Env: "MARGINCE_METRICS_TOKEN", Type: config.String, Secret: true, Doc: "d"})
	cfg, err := load(t, fixture{env: []string{"MARGINCE_DSN=postgres://x"}}, optional, dsn)
	if err != nil {
		t.Fatalf("an unset optional secret failed the load: %v", err)
	}
	if v, origin := config.ValWithSource(cfg, optional); v != "" || origin.Source != config.SourceUnset {
		t.Errorf("got %q from %s, want empty and unset", v, origin.Source)
	}
	if _, err := load(t, fixture{}, optional, dsn); err == nil || !strings.Contains(err.Error(), "MARGINCE_DSN: required") {
		t.Errorf("an unset NonEmpty setting was not a fault: %v", err)
	}
}

// A check that reads two settings runs beside the faults of each one, and sees
// a faulted setting as unset rather than panicking on it.
func TestCrossSettingFaultsJoinTheLoadFaults(t *testing.T) {
	reg, src := sources(t, fixture{args: []string{"--log-level=debug"}}, level, dsn)
	var dsnOrigin config.Source
	src.Faults = func(c *config.Config) []error {
		_, origin := config.ValWithSource(c, dsn)
		dsnOrigin = origin.Source
		if config.Val(c, level) == "debug" {
			return []error{errors.New("debug needs a second setting")}
		}
		return nil
	}
	_, err := config.Load(reg, src)
	for _, want := range []string{"MARGINCE_DSN: required", "debug needs a second setting"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if dsnOrigin != config.SourceUnset {
		t.Errorf("a faulted setting read from %q, want unset", dsnOrigin)
	}
}

func TestOnlyAFlagOriginIsFromTheFlag(t *testing.T) {
	if !(config.Origin{Source: config.SourceFlag}).FromFlag() || (config.Origin{Source: config.SourceEnv}).FromFlag() {
		t.Fatal("FromFlag must hold for a flag origin and only for it")
	}
}

func TestYAMLSourcesUseTheConfiguredFileReader(t *testing.T) {
	base := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(base, []byte("workspace: {name: Demo}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		args    []string
		files   map[string]string
		want    string
		refused bool
	}{
		{"absent base stays unset", nil, nil, "info", false},
		{"base comes from the reader", nil, map[string]string{base: "log: {level: debug}\n"}, "debug", false},
		{"overlay comes from the reader", []string{"--config-file", "override.yaml"}, map[string]string{base: "log: {level: warn}\n", "override.yaml": "log: {level: debug}\n"}, "debug", false},
		{"missing explicit overlay is refused", []string{"--config-file", "override.yaml"}, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg, err := config.NewRegistry([]config.Any{level})
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{"--config", base}, tc.args...)
			loaded, err := config.Load(reg, config.Sources{Args: args, ReadFile: func(path string) ([]byte, error) {
				body, ok := tc.files[path]
				if !ok {
					return nil, os.ErrNotExist
				}
				return []byte(body), nil
			}})
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "override.yaml") {
					t.Fatalf("missing overlay was not refused: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := config.Val(loaded, level); got != tc.want {
				t.Fatalf("level %q, want %q", got, tc.want)
			}
		})
	}
}
