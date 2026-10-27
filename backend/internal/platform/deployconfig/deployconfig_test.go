// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

const fullConfig = `
version: 1
workspace:
  name: Gradion
  base_currency: EUR
  timezone: Europe/Berlin
bootstrap_admin:
  email: lars@example.com
  display_name: Lars
  password_file: /run/secrets/admin-password
seeds:
  pipeline:
    name: Sales
    stages:
      - { name: Qualified, probability: 10 }
      - { name: Proposal, probability: 40 }
  consent_purposes:
    - { key: marketing_email, label: Marketing email, double_opt_in: true }
  starter_automations: false
auth:
  password:
    enabled: true
email:
  enabled: true
  smtp:
    host: smtp.example.com
    port: 587
    username: crm@example.com
  from_address: crm@example.com
company_context:
  rollout: tasks
`

func TestParseAcceptsTheFullDocumentedShape(t *testing.T) {
	cfg, err := LoadBootstrap(writeTemp(t, fullConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.Name != "Gradion" || cfg.BootstrapAdmin.Email != "lars@example.com" {
		t.Fatalf("parsed company/admin = %+v / %+v", cfg.Workspace, cfg.BootstrapAdmin)
	}
	if cfg.Seeds.Pipeline.Name != "Sales" || len(cfg.Seeds.Pipeline.Stages) != 2 {
		t.Fatalf("parsed pipeline seed = %+v", cfg.Seeds.Pipeline)
	}
	if cfg.Seeds.StarterAutomations == nil || *cfg.Seeds.StarterAutomations {
		t.Fatal("starter_automations: false did not parse")
	}
}

func TestParseValidatesFailClosed(t *testing.T) {
	cases := map[string]string{ // #nosec G101 -- yaml documents that must FAIL validation, not credentials
		"unsupported version":     "version: 2\n",
		"bad timezone":            "version: 1\nworkspace: { name: X, timezone: Mars/Olympus }\n",
		"bad currency":            "version: 1\nworkspace: { name: X, base_currency: euros }\n",
		"upper-case country":      "version: 1\nworkspace: { name: X, country: DE }\n",
		"three-letter country":    "version: 1\nworkspace: { name: X, country: deu }\n",
		"admin without password":  "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A }\n",
		"inline secret refused":   "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A, password: hunter2hunter2 }\n",
		"empty pipeline":          "version: 1\nseeds: { pipeline: { name: Sales, stages: [] } }\n",
		"duplicate stage":         "version: 1\nseeds: { pipeline: { name: S, stages: [ { name: A, probability: 10 }, { name: A, probability: 20 } ] } }\n",
		"probability out of band": "version: 1\nseeds: { pipeline: { name: S, stages: [ { name: A, probability: 140 } ] } }\n",
		"purpose without label":   "version: 1\nseeds: { consent_purposes: [ { key: marketing_email } ] }\n",
	}
	for name, doc := range cases {
		if _, err := LoadBootstrap(writeTemp(t, doc)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func TestLoadMissingFileBootsExistingInstallation(t *testing.T) {
	cfg, err := LoadBootstrap(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("missing file must not be an error (it cannot bootstrap, only bind): %v", err)
	}
	if cfg.BootstrapAdmin != nil {
		t.Fatalf("zero config = %+v, want no bootstrap admin", cfg)
	}
}

// The annotated example shipped for operators must always satisfy the real
// parser — a schema change here without a matching edit there would hand every
// new deployment (and every `make dev`, which copies it) a config that fails at
// boot. It also guards the now-active rates block against a typo.
func TestShippedExampleConfigParses(t *testing.T) {
	if _, err := LoadBootstrap("../../../../config/margince.example.yaml"); err != nil {
		t.Fatalf("config/margince.example.yaml no longer parses: %v", err)
	}
}

// writeTemp writes doc to a fresh file under t.TempDir() and returns its
// path, for tests that exercise Load (rather than Parse) against a real
// file on disk.
func writeTemp(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

// Lists ship on: an installation whose files never mention them has them, and
// an operator who writes `enabled: false` in either file switches them off.
// The retention posture defaults to the historical behaviour byte for byte: an
// omitted block, an empty value and the explicit `standard` all leave storage
// limitation enforcing. An installation under a keep-everything obligation is
// the one that has to say so, and only `retain_only` says it.
func TestRetentionSeedPostureDefaultsToStandard(t *testing.T) {
	for name, doc := range map[string]string{
		"block omitted":     "version: 1\n",
		"seeds without it":  "version: 1\nseeds:\n  starter_automations: false\n",
		"empty value":       "version: 1\nseeds:\n  retention:\n    default_policy: \"\"\n",
		"explicit standard": "version: 1\nseeds:\n  retention:\n    default_policy: standard\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := LoadBootstrap(writeTemp(t, doc))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if cfg.Seeds.RetainOnly() {
				t.Error("the retain-only posture was seeded ON without the operator asking; " +
					"a fresh installation must enforce storage limitation (Art. 5(1)(e)) out of the box")
			}
		})
	}
}

func TestRetentionSeedRetainOnlyTurnsThePostureOn(t *testing.T) {
	cfg, err := LoadBootstrap(writeTemp(t, "version: 1\nseeds:\n  retention:\n    default_policy: retain_only\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Seeds.Retention == nil || cfg.Seeds.Retention.DefaultPolicy != RetentionRetainOnlyPosture {
		t.Fatalf("seeds.retention did not parse: %+v", cfg.Seeds.Retention)
	}
	if !cfg.Seeds.RetainOnly() {
		t.Error("default_policy: retain_only did not seed the posture; the nightly pass could destroy " +
			"records between bootstrap and the first admin login, which is the window this key closes")
	}
}

// A typo must not fall back to `standard` in silence: for the one deployment
// that set the key on purpose, silence is the exact failure it set it to prevent.
func TestRetentionSeedRefusesAnUnknownPosture(t *testing.T) {
	for _, value := range []string{"retain-only", "retainonly", "keep_everything", "Standard"} {
		_, err := LoadBootstrap(writeTemp(t, "version: 1\nseeds:\n  retention:\n    default_policy: "+value+"\n"))
		if err == nil {
			t.Fatalf("Parse accepted seeds.retention.default_policy: %q", value)
		}
		for _, want := range []string{"seeds.retention.default_policy", value, RetentionStandardPosture, RetentionRetainOnlyPosture} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal for %q omits %q, so the operator cannot fix the file from it: %v", value, want, err)
			}
		}
	}
}

func TestBootstrapAdminPasswordComesFromTheFileReference(t *testing.T) {
	pwFile := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(pwFile, []byte("a bootstrap password!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The original spelling still works: an existing deployment must boot
	// unchanged now that the reference form sits beside it.
	b := BootstrapAdmin{Email: "a@b.co", DisplayName: "A", PasswordFile: pwFile}
	pw, err := b.ResolvePassword(config.Static(nil))
	if err != nil {
		t.Fatalf("ResolvePassword: %v", err)
	}
	if pw != "a bootstrap password!" {
		t.Fatalf("password = %q, want the file content without the trailing newline", pw)
	}

	if err := os.WriteFile(pwFile, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ResolvePassword(config.Static(nil)); err == nil {
		t.Fatal("an under-12-character bootstrap password was accepted")
	}
}

// The reference form, and which spelling wins when a deployment carries both.
func TestBootstrapAdminPasswordComesFromEitherSpelling(t *testing.T) {
	dir := t.TempDir()
	fromFile := filepath.Join(dir, "from-file")
	if err := os.WriteFile(fromFile, []byte("the file password!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := config.Static(map[string]string{"PROBE_ADMIN_PW": "the environment password!"})

	var withRef struct {
		B BootstrapAdmin `yaml:"b"`
	}
	doc := "b:\n  password: ${env:PROBE_ADMIN_PW}\n"
	if err := yaml.Unmarshal([]byte(doc), &withRef); err != nil {
		t.Fatal(err)
	}
	got, err := withRef.B.ResolvePassword(env)
	if err != nil {
		t.Fatalf("resolving the reference form: %v", err)
	}
	if got != "the environment password!" {
		t.Errorf("password = %q, want the environment value", got)
	}

	// Both present: the reference wins, because it is the form that can name
	// the environment and the one an operator migrating forward added on
	// purpose.
	withRef.B.PasswordFile = fromFile
	got, err = withRef.B.ResolvePassword(env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "the environment password!" {
		t.Errorf("with both spellings the password was %q; the reference must win", got)
	}
}

// A deployment naming no password at all is told so, rather than bootstrapping
// an account with an empty credential.
func TestBootstrapAdminWithNoPasswordSourceRefuses(t *testing.T) {
	_, err := BootstrapAdmin{Email: "a@b.co", DisplayName: "A"}.ResolvePassword(config.Static(nil))
	if err == nil {
		t.Fatal("an admin with no password source was accepted")
	}
	if !strings.Contains(err.Error(), "${file:") || !strings.Contains(err.Error(), "${env:") {
		t.Errorf("the refusal does not show how to supply one: %v", err)
	}
}

// Every shipped `seeds.ai_routing` block decodes through the parser every boot
// runs, and survives the re-encoding that hands it to the ai parser. A field
// declared `*yaml.Node` is decoded as an ordinary struct, and every program
// then refuses to start with "field profile not found in type yaml.Node".
func TestEveryShippedRoutingSeedDecodesThroughTheBootstrap(t *testing.T) {
	paths, err := filepath.Glob("../../../../config/presets/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../../../../config/margince.dev.yaml")
	if len(paths) < 2 {
		t.Fatal("no config/presets/*.yaml found; the presets moved and this test checks the dev file alone")
	}
	for _, path := range paths {
		cfg, err := LoadBootstrap(path)
		if err != nil {
			t.Errorf("%s: the seeds do not parse: %v", path, err)
			continue
		}
		if cfg.Seeds.AIRouting.IsZero() {
			t.Errorf("%s: the seeds parsed but carried no binding", path)
			continue
		}
		out, err := yaml.Marshal(&cfg.Seeds.AIRouting)
		if err != nil {
			t.Errorf("%s: re-encoding the binding: %v", path, err)
			continue
		}
		for _, want := range []string{"profile:", "tiers:", "embeddings:"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: the re-encoded binding lost %q:\n%s", path, want, out)
			}
		}
	}
}

// The reference form the field's own doc comment says to prefer is ACCEPTED.
//
// validate checked password_file alone, so a deployment that wrote `password`
// was refused at first boot with a message naming the legacy spelling — the
// natural read being that the reference form is unsupported, with nothing to
// correct it. It also made Parse and ResolvePassword two different contracts
// for one field: Parse refused a document the resolver handles perfectly.
func TestParseAcceptsEitherSpellingOfTheAdminPassword(t *testing.T) {
	both := map[string]string{
		"the reference form": "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A, password: \"${file:/run/secrets/admin-password}\" }\n",
		"the env reference":  "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A, password: \"${env:MARGINCE_ADMIN_PASSWORD}\" }\n",
		"the original file":  "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A, password_file: /run/secrets/admin-password }\n",
	}
	for name, doc := range both {
		if _, err := LoadBootstrap(writeTemp(t, doc)); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}

	// And the two refusals stay refusals: an INLINE value is still not a
	// reference, and naming no password at all is still an error. A fix that
	// widened the check into accepting anything would pass the cases above
	// while removing what they are a relaxation of.
	for name, doc := range map[string]string{ // #nosec G101 -- documents that must FAIL, not credentials
		"an inline password": "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A, password: hunter2hunter2 }\n",
		"neither spelling":   "version: 1\nbootstrap_admin: { email: a@b.co, display_name: A }\n",
	} {
		if _, err := LoadBootstrap(writeTemp(t, doc)); err == nil {
			t.Errorf("%s parsed without error", name)
		}
	}
}

func TestAValidCountryParsesAsWritten(t *testing.T) {
	cfg, err := LoadBootstrap(writeTemp(t, "version: 1\nworkspace: { name: X, country: de }\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Workspace.Country != "de" {
		t.Fatalf("workspace.country = %q, want de", cfg.Workspace.Country)
	}
}
