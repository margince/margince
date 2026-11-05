// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The boot flags this process refuses to guess at. A configuration value that
// the code would reject later has to be rejected HERE, while an operator is
// still watching a terminal, rather than on the first request that needs it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/programsettings"
)

const testDSN = "postgres://localhost/margince_test"

// The connector publishes --public-base-url verbatim as its OAuth audience, its
// RFC 9728 protected-resource document and its advertised MCP URL. A value that
// is wrong in any of those ways must fail while an operator is watching, not
// boot a connector advertising somewhere nobody can reach.
func TestValidatePublicBaseURLRefusesAnythingButABareOrigin(t *testing.T) {
	for _, ok := range []string{
		"https://crm.example.com",
		"http://localhost:8080",
		// A trailing slash means the same origin, and the /mcp derivation trims
		// it, so it is accepted rather than nitpicked.
		"https://crm.example.com/",
	} {
		if err := validatePublicBaseURL(ok); err != nil {
			t.Errorf("validatePublicBaseURL(%q) = %v, want nil", ok, err)
		}
	}

	for _, bad := range []struct{ name, raw string }{
		{"no scheme", "crm.example.com"},
		{"a scheme nothing dereferences", "ftp://crm.example.com"},
		{"no host", "https://"},
		// The MCP resource is this value + "/mcp", so a path here publishes
		// something like https://host/base/mcp while the route is at /mcp.
		{"a path", "https://crm.example.com/base"},
		{"a query", "https://crm.example.com?x=1"},
		{"a fragment", "https://crm.example.com#f"},
		// ":8080" is a non-empty authority that names no host — Host keeps the
		// port, so only Hostname() catches it.
		{"a hostless authority", "http://:8080"},
		// url.Parse decodes as it goes, so both of these arrive as a path that
		// trims to empty while the RAW value is what gets published.
		{"a repeated separator", "https://crm.example.com//"},
		{"an encoded separator", "https://crm.example.com/%2F"},
	} {
		t.Run(bad.name, func(t *testing.T) {
			err := validatePublicBaseURL(bad.raw)
			if err == nil {
				t.Fatalf("validatePublicBaseURL(%q) = nil, want a refusal", bad.raw)
			}
			// The operator has to be able to fix it from the message alone.
			if !strings.Contains(err.Error(), bad.raw) {
				t.Errorf("refusal %q does not quote the offending value", err)
			}
		})
	}
}

// An origin carrying userinfo is refused WITHOUT quoting it. Every other
// refusal quotes the value so an operator can fix it, but this one would copy a
// password into the boot error and every log line that carries it.
func TestValidatePublicBaseURLRefusesUserinfoWithoutEchoingIt(t *testing.T) {
	const secret = "s3cr3t-password"
	err := validatePublicBaseURL("https://admin:" + secret + "@crm.example.com")
	if err == nil {
		t.Fatal("an origin with userinfo was accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the refusal leaked the credential: %q", err)
	}
	if !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("refusal %q does not say what is wrong", err)
	}
}

// Every configuration fault this layer can see is reported together.
//
// Starting the binary by hand used to be a guessing game played one boot at a
// time: the first return said only what it had reached, and the next run
// answered with a fault that had been true all along.
func TestParseReportsEveryConfigurationFaultAtOnce(t *testing.T) {
	t.Parallel()
	_, _, err := loadAPIConfig([]string{"--metrics-access", "sometimes"}, nil)
	if err == nil {
		t.Fatal("parsing answered no error, want both faults reported")
	}
	for _, want := range []string{"MARGINCE_DSN", "--metrics-access"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s — an operator fixing one fault at a time "+
				"pays a boot per fault:\n%s", want, err)
		}
	}
}

// One fault still reads as one sentence: a list of one is a worse message than
// the sentence it replaces.
func TestOneFaultIsNotRenderedAsAList(t *testing.T) {
	t.Parallel()
	_, _, err := loadAPIConfig(nil, nil)
	if err == nil {
		t.Fatal("parsing answered no error, want the missing DSN")
	}
	if strings.Contains(err.Error(), "\n  - ") {
		t.Errorf("a single fault was rendered as a bullet list:\n%s", err)
	}
}

// The boot line names BOTH halves of an environment app, and neither when there
// is no environment app at all.
//
// The three states are not two: an installation with no Entra app in the
// environment has not omitted flags, it declined them, and telling it what is
// "missing" sends an operator to supply credentials they deliberately keep in
// Settings. Either flag ALONE is somebody part-way through, and which half is
// absent is the whole content of the message.
func TestTheBootLineNamesWhichHalfOfTheEnvironmentAppIsMissing(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		cfg  apiConfig
		want string
	}{
		"no environment app":  {apiConfig{}, ""},
		"complete":            {apiConfig{graphClientID: "id", graphClientSecret: "s"}, ""},
		"id without a secret": {apiConfig{graphClientID: "id"}, "--graph-client-secret"},
		"secret without an id": {
			apiConfig{graphClientSecret: "s"}, "--graph-client-id",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := envAppShortfall(tc.cfg)
			if tc.want == "" {
				if got != "" {
					t.Errorf("shortfall = %q, want nothing said", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("shortfall = %q, want it to name %s", got, tc.want)
			}
		})
	}
}

// A LIST BELONGS TO SIGN-IN, and --graph-tenant is spliced into a URL.
//
// Capture builds login.microsoftonline.com/%s/oauth2/... out of this value, so
// a comma produces an authority no directory answers for — and the failure
// arrives at Microsoft's consent screen, in Microsoft's words, long after boot.
// Its sibling takes several directories and falls back to this flag, which is
// exactly why somebody would put the list here.
func TestTheCaptureTenantRefusesAListAndSaysWhereItGoes(t *testing.T) {
	t.Parallel()
	_, _, err := loadAPIConfig([]string{
		"--dsn", "postgres://x/y",
		"--graph-tenant", "11111111-2222-3333-4444-555555555555,9188040d-6c67-4c5b-b112-36a304b66dad",
	}, nil)
	if err == nil {
		t.Fatal("a list of directories was accepted as the capture authority")
	}
	if !strings.Contains(err.Error(), "--microsoft-signin-tenant") {
		t.Errorf("the refusal reads %q, want it to name the flag that does take a list", err)
	}

	// ONE authority still boots, including the alias capture defaults to.
	for _, tenant := range []string{"common", "11111111-2222-3333-4444-555555555555"} {
		if _, _, err := loadAPIConfig([]string{"--dsn", "postgres://x/y", "--graph-tenant", tenant}, nil); err != nil {
			t.Errorf("--graph-tenant %s was refused: %v", tenant, err)
		}
	}
}

// /metrics is closed unless an operator says otherwise, and the two ways of
// saying so cannot be combined: an open endpoint given a token as well would
// authenticate nothing while the operator believed it did.
func TestMetricsAccessIsTokenByDefaultAndRefusesAContradiction(t *testing.T) {
	t.Run("unset is token", func(t *testing.T) {
		cfg, _, err := loadAPIConfig([]string{"--dsn", testDSN}, nil)
		if err != nil {
			t.Fatalf("parsing: %v", err)
		}
		if cfg.metricsAccess != programsettings.MetricsAccessToken {
			t.Errorf("metricsAccess = %q, want %q", cfg.metricsAccess, programsettings.MetricsAccessToken)
		}
	})

	t.Run("the environment opens it", func(t *testing.T) {
		cfg, _, err := loadAPIConfig([]string{"--dsn", testDSN}, []string{"MARGINCE_METRICS_ACCESS=open"})
		if err != nil {
			t.Fatalf("parsing: %v", err)
		}
		if cfg.metricsAccess != programsettings.MetricsAccessOpen {
			t.Errorf("metricsAccess = %q, want %q", cfg.metricsAccess, programsettings.MetricsAccessOpen)
		}
	})

	t.Run("a token with the default posture boots", func(t *testing.T) {
		if _, _, err := loadAPIConfig([]string{"--dsn", testDSN, "--metrics-token", "stands-in-for-a-token"}, nil); err != nil {
			t.Fatalf("parsing: %v", err)
		}
	})

	t.Run("open with a token fails the boot", func(t *testing.T) {
		_, _, err := loadAPIConfig([]string{"--dsn", testDSN, "--metrics-access", "open", "--metrics-token", "stands-in-for-a-token"}, nil)
		if err == nil || !strings.Contains(err.Error(), "--metrics-access=open") {
			t.Fatalf("err = %v, want a boot error naming --metrics-access=open", err)
		}
		if strings.Contains(err.Error(), "stands-in-for-a-token") {
			t.Errorf("the boot error echoed the token: %v", err)
		}
	})

	t.Run("an unknown posture fails the boot", func(t *testing.T) {
		_, _, err := loadAPIConfig([]string{"--dsn", testDSN, "--metrics-access", "public"}, nil)
		if err == nil || !strings.Contains(err.Error(), "--metrics-access") || !strings.Contains(err.Error(), "token, open") {
			t.Fatalf("err = %v, want a boot error naming the flag and both postures", err)
		}
	})
}

// The trusted-proxy list is a boot fault when it cannot be honoured safely,
// never a silent empty set: an operator who set it believes the limits key on
// the client, and an ignored value would leave them keyed on the proxy.
func TestLoadAPIConfig_TrustedProxies(t *testing.T) {
	t.Run("unset trusts nobody", func(t *testing.T) {
		cfg, _, err := loadAPIConfig([]string{"--dsn", testDSN}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.trustedProxies.Empty() {
			t.Fatalf("default must trust nobody; got %s", cfg.trustedProxies)
		}
	})
	t.Run("env value is parsed", func(t *testing.T) {
		cfg, _, err := loadAPIConfig([]string{"--dsn", testDSN}, []string{"MARGINCE_TRUSTED_PROXIES=10.0.0.0/16"})
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.trustedProxies.String(); got != "10.0.0.0/16" {
			t.Fatalf("got %q", got)
		}
	})
	for _, bad := range []string{"0.0.0.0/0", "ingress-nginx"} {
		t.Run("refuses "+bad, func(t *testing.T) {
			_, _, err := loadAPIConfig([]string{"--dsn", testDSN, "--trusted-proxies", bad}, nil)
			// The parser's reason quotes the entry, so an operator can fix it.
			if err == nil || !strings.Contains(err.Error(), "--trusted-proxies") || !strings.Contains(err.Error(), bad) {
				t.Fatalf("want a --trusted-proxies boot fault quoting %q, got %v", bad, err)
			}
		})
	}
}

// The faults Load finds and the ones a single setting's own check finds arrive
// in one error, so a boot names all of them.
func TestTheAPIReportsEveryConfigurationFaultTogether(t *testing.T) {
	t.Parallel()
	_, _, err := loadAPIConfig([]string{"--metrics-access=open", "--metrics-token=x", "--graph-tenant=a,b"}, nil)
	for _, want := range []string{"MARGINCE_DSN: required", "--graph-tenant takes ONE authority", "--metrics-access=open serves /metrics"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

// A fake model left set in a production environment would serve canned
// answers. A command line names it for that one process.
func TestTheFakeModelStaysOutOfAProductionEnvironment(t *testing.T) {
	t.Parallel()
	dsnEnv := "MARGINCE_DSN=" + testDSN
	_, _, err := loadAPIConfig(nil, []string{dsnEnv, "MARGINCE_AI_FAKE=true", "MARGINCE_ENV=production"})
	if err == nil || !strings.Contains(err.Error(), "MARGINCE_AI_FAKE") {
		t.Fatalf("err = %v, want a boot fault naming MARGINCE_AI_FAKE", err)
	}
	for name, tc := range map[string]struct{ args, environ []string }{
		"set in a dev environment":       {nil, []string{dsnEnv, "MARGINCE_AI_FAKE=true", "MARGINCE_ENV=dev"}},
		"passed on a production command": {[]string{"--ai-fake"}, []string{dsnEnv, "MARGINCE_ENV=production"}},
	} {
		cfg, _, err := loadAPIConfig(tc.args, tc.environ)
		if err != nil || !cfg.fakeBrain {
			t.Errorf("%s: fake %v, err %v; want the fake model allowed", name, cfg.fakeBrain, err)
		}
	}
}

// One margince.yaml serves the api and the worker, so a key only the worker
// reads must not fail the api's boot.
func TestTheAPIAcceptsAYAMLKeyOnlyTheWorkerReads(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte("job_drain_window: 45s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadAPIConfig([]string{"--config", path, "--dsn", testDSN}, nil); err != nil {
		t.Fatal(err)
	}
}

// An old per-posture overlay beside the base file is no longer read. Booting
// past it would drop whatever it armed without a word, so the boot stops and
// says how to keep it.
func TestAnOverlayBesideTheBaseFileStopsTheBoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "margince.yaml")
	mustWrite(t, base, "version: 1\n")
	mustWrite(t, filepath.Join(dir, "margince.dev.yaml"), "version: 1\n")
	_, _, err := loadAPIConfig([]string{"--config", base, "--dsn", "x"}, []string{"MARGINCE_ENV=dev"})
	if err == nil || !strings.Contains(err.Error(), "overlay files are no longer read; pass it with --config-file") {
		t.Fatalf("got %v", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
