// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package cliflags

import (
	"flag"
	"strings"
	"testing"
	"time"
)

// fakeEnv is a getenv the test controls, so no case depends on the process
// environment and none can leak into a sibling.
func fakeEnv(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

// The usage text must never carry an environment value. This is the reason the
// package exists: flag renders a non-empty default as `(default "…")`, so a
// credential wired into a default reaches stderr on any parse error.
func TestUsageTextCarriesNoEnvironmentValue(t *testing.T) {
	var env Env
	var dsn, cfg string
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var usage strings.Builder
	fs.SetOutput(&usage)

	env.String(fs, &dsn, "dsn", "PROBE_DSN", "", "Postgres DSN")
	env.String(fs, &cfg, "config", "PROBE_CONFIG", "probe.yaml", "path to the config file")
	if err := env.Apply(fs, fakeEnv(map[string]string{
		"PROBE_DSN":    "postgres://u:SUPERSECRET@host/db",
		"PROBE_CONFIG": "/etc/probe.yaml",
	})); err != nil {
		t.Fatal(err)
	}
	fs.PrintDefaults()

	if strings.Contains(usage.String(), "SUPERSECRET") {
		t.Errorf("the usage text carries the environment's value:\n%s", usage.String())
	}
	// The literal IS echoed on purpose — it tells an operator what happens with no
	// configuration at all, and it is not a secret.
	if !strings.Contains(usage.String(), "probe.yaml") {
		t.Errorf("the literal default is missing, so the usage text stopped being useful:\n%s", usage.String())
	}
}

func TestPrecedenceIsFlagThenEnvironmentThenLiteral(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  string
		want string
	}{
		{"the flag wins over the environment", []string{"--config", "/from/flag"}, "/from/env", "/from/flag"},
		{"the environment wins over the literal", nil, "/from/env", "/from/env"},
		{"the literal is the last resort", nil, "", "probe.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var env Env
			var cfg string
			fs := flag.NewFlagSet("probe", flag.ContinueOnError)
			env.String(fs, &cfg, "config", "PROBE_CONFIG", "probe.yaml", "path")
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("parsing %v: %v", tc.args, err)
			}
			if err := env.Apply(fs, fakeEnv(map[string]string{"PROBE_CONFIG": tc.env})); err != nil {
				t.Fatal(err)
			}

			if cfg != tc.want {
				t.Errorf("config resolved to %q, want %q", cfg, tc.want)
			}
		})
	}
}

// An explicitly passed empty flag stays empty. Treating it as absent would let a
// wrapper's `--flag "$UNSET_VAR"` silently pick up an ambient value instead.
func TestAnExplicitEmptyFlagIsNotFilledFromTheEnvironment(t *testing.T) {
	var env Env
	var cfg string
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	env.String(fs, &cfg, "config", "PROBE_CONFIG", "probe.yaml", "path")
	if err := fs.Parse([]string{"--config", ""}); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if err := env.Apply(fs, fakeEnv(map[string]string{"PROBE_CONFIG": "/from/env"})); err != nil {
		t.Fatal(err)
	}

	if cfg != "" {
		t.Errorf("an explicit empty --config became %q; the caller's intent was overridden", cfg)
	}
}

// An empty environment value must not erase a literal default: .env.example
// promises a blank line is the same as unset, and env files are sourced wholesale.
func TestAnEmptyEnvironmentValueLeavesTheLiteralAlone(t *testing.T) {
	var env Env
	var cfg string
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	env.String(fs, &cfg, "config", "PROBE_CONFIG", "probe.yaml", "path")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if err := env.Apply(fs, fakeEnv(map[string]string{"PROBE_CONFIG": ""})); err != nil {
		t.Fatal(err)
	}

	if cfg != "probe.yaml" {
		t.Errorf("a blank environment value erased the literal default, leaving %q", cfg)
	}
}

func TestEnvKeysNamesEveryBinding(t *testing.T) {
	var env Env
	var a, b string
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	env.String(fs, &a, "one", "PROBE_ONE", "", "")
	env.String(fs, &b, "two", "PROBE_TWO", "", "")

	keys := strings.Join(env.EnvKeys(), ",")
	if keys != "PROBE_ONE,PROBE_TWO" {
		t.Errorf("EnvKeys returned %q; a test that seeds from this would miss a binding", keys)
	}
}

// TestItemsCarryNoEnvironmentValue is the mirror of the usage-text gate above,
// for the other artefact this package now feeds.
//
// Items publishes each flag's DefValue, and a generated template or schema
// renders that. If a registration ever took its default FROM the environment,
// the value would travel there exactly as it once travelled into usage text —
// same leak, new destination.
func TestItemsCarryNoEnvironmentValue(t *testing.T) {
	const sentinel = "a-value-the-environment-supplied"
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var env Env
	var dsn, level string
	env.String(fs, &dsn, "dsn", "PROBE_DSN", "", "the DSN")
	env.String(fs, &level, "log-level", "PROBE_LOG_LEVEL", "info", "log level")

	// Everything the environment could supply, supplied.
	if err := env.Apply(fs, func(string) string { return sentinel }); err != nil {
		t.Fatal(err)
	}

	for _, item := range env.Items(fs, "probe", map[string]bool{"PROBE_LOG_LEVEL": true}) {
		if strings.Contains(item.Default, sentinel) {
			t.Errorf("%s carries an environment-supplied default %q into the declared surface", item.Name, item.Default)
		}
	}
}

// A binding nobody classified is withheld, not published: the map miss must
// fail closed, because the recoverable mistake is a redacted value an operator
// asks about and the unrecoverable one is a bearer token in a build log.
func TestAnUnclassifiedBindingIsTreatedAsASecret(t *testing.T) {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var env Env
	var newKnob string
	env.String(fs, &newKnob, "new-knob", "PROBE_NEW_KNOB", "", "a knob nobody classified")

	items := env.Items(fs, "probe", map[string]bool{})
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if !items[0].Secret {
		t.Error("an unclassified binding was published; the default must be to withhold")
	}
}

// A typed flag takes its environment value through its own kind, with the
// same precedence a string flag has, and a value the kind cannot read is a
// fault naming the variable rather than a quiet fall back to the literal.
func TestATypedFlagReadsItsKindAndRefusesWhatItCannot(t *testing.T) {
	var env Env
	var wait time.Duration
	var count int
	var on bool
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	env.Duration(fs, &wait, "wait", "PROBE_WAIT", time.Minute, "a wait")
	env.Int(fs, &count, "count", "PROBE_COUNT", 3, "a count")
	env.Bool(fs, &on, "on", "PROBE_ON", true, "a switch")
	if err := fs.Parse([]string{"--count", "9"}); err != nil {
		t.Fatal(err)
	}
	if err := env.Apply(fs, fakeEnv(map[string]string{"PROBE_WAIT": "90s", "PROBE_COUNT": "4", "PROBE_ON": "false"})); err != nil {
		t.Fatal(err)
	}
	if wait != 90*time.Second || count != 9 || on {
		t.Errorf("resolved wait=%v count=%d on=%v, want 90s from the environment, 9 from the flag, false from the environment", wait, count, on)
	}

	err := env.Apply(fs, fakeEnv(map[string]string{"PROBE_WAIT": "soon", "PROBE_ON": "yes"}))
	if err == nil {
		t.Fatal("unreadable values were accepted")
	}
	for _, name := range []string{"PROBE_WAIT", "PROBE_ON"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not name %s, so a boot would report only one of two faults:\n%v", name, err)
		}
	}
}

func TestShortfallsNamesEveryWayAFlagEscapesTheEnvironment(t *testing.T) {
	var env Env
	var bound, oddly, loose, dev string
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	// Built from Namespace rather than spelled: these are fixtures, not
	// variables an operator can set, and the env-contract gate reads literals.
	env.String(fs, &bound, "bound", Namespace+"BOUND", "", "fine")
	env.String(fs, &oddly, "oddly", Namespace+"ODD", "", "misnamed")
	fs.StringVar(&loose, "loose", "", "no variable")
	fs.StringVar(&dev, "dev", "", "kept off on purpose")

	got := strings.Join(env.Shortfalls(fs, func(name string) bool { return name == "dev" }), "\n")
	for _, want := range []string{"--oddly is read from " + Namespace + "ODD", "--loose has no environment variable"} {
		if !strings.Contains(got, want) {
			t.Errorf("shortfalls do not say %q:\n%s", want, got)
		}
	}
	for _, clean := range []string{"--bound", "--dev"} {
		if strings.Contains(got, clean+" ") {
			t.Errorf("shortfalls name %s, which is bound or kept off with a reason:\n%s", clean, got)
		}
	}
}
