// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"flag"
	"strings"
	"testing"
)

func TestFlagsAreTheRegisteredOnesPlusTheLoaderOwn(t *testing.T) {
	reg := mustRegistry(t,
		Define(Spec[string]{Key: "addr", Env: "X_ADDR", Flag: "addr", Type: String, Doc: "d"}),
		Define(Spec[bool]{Key: "inline_relay", Env: "X_INLINE_RELAY", Flag: "inline-relay", Type: Bool, Doc: "d"}),
	)
	got, err := parseFlags(reg, []string{"--addr=:9", "--config", "a.yaml", "--config-file=b.yaml", "--env-file", "c.env", "--config-trace", "--inline-relay"})
	if err != nil {
		t.Fatal(err)
	}
	if got.raw["addr"] != ":9" || got.config != "a.yaml" || got.configFile != "b.yaml" || got.envFile != "c.env" || !got.trace {
		t.Fatalf("got %+v", got)
	}
	if got.raw["inline-relay"] != "true" {
		t.Fatalf("a bare bool flag means true, got %q", got.raw["inline-relay"])
	}
	if _, err := parseFlags(reg, []string{"--adr=:9"}); err == nil {
		t.Fatal("an unknown flag must stop the boot")
	}
}

func TestFlagsRefuseStrayArguments(t *testing.T) {
	reg := mustRegistry(t, Define(Spec[bool]{Key: "inline_relay", Env: "X_INLINE_RELAY", Flag: "inline-relay", Type: Bool, Doc: "d"}))
	_, err := parseFlags(reg, []string{"--inline-relay", "false", "--addr=:9"})
	if err == nil || err.Error() != "unexpected argument at position 2; flags take --name=value" {
		t.Fatalf("got %v", err)
	}
}

func mustRegistry(t *testing.T, s ...Any) *Registry {
	t.Helper()
	r, err := NewRegistry(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUsageNamesEveryFlagAndNoDefault(t *testing.T) {
	reg := mustRegistry(t,
		Define(Spec[string]{Key: "addr", Env: "X_ADDR", Flag: "addr", Type: String, Default: Some(":8080"), Doc: "listen address"}),
		Define(Spec[string]{Key: "token", Env: "X_TOKEN", Flag: "token", Type: String, Secret: true, Doc: "a bearer"}),
	)
	var out strings.Builder
	PrintUsage(&out, reg)
	for _, want := range []string{"-addr", "listen address", "-token", "a bearer", "-config-file"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("usage lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), ":8080") {
		t.Errorf("usage prints a default:\n%s", out.String())
	}
}

func TestACommandsOwnFlagsLeaveTheSettingsToLoad(t *testing.T) {
	dsn := Define(Spec[string]{Key: "dsn", Env: "X_DSN", Flag: "dsn", Type: String, Doc: "d"})
	fake := Define(Spec[bool]{Key: "fake", Env: "X_FAKE", Flag: "fake", Type: Bool, Doc: "d"})
	reg := mustRegistry(t, dsn, fake)
	fs := flag.NewFlagSet("cmd", flag.ContinueOnError)
	steps := fs.Int("steps", 1, "own flag")
	settings := AddFlags(fs, reg)
	if err := fs.Parse([]string{"--steps", "3", "--dsn", "from-flag", "--fake"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(reg, Sources{Args: settings.Args(), Environ: []string{"X_DSN=from-env"}})
	if err != nil {
		t.Fatal(err)
	}
	if *steps != 3 || Val(cfg, dsn) != "from-flag" || !Val(cfg, fake) {
		t.Fatalf("steps %d, settings %v", *steps, cfg.TraceLines())
	}
}
