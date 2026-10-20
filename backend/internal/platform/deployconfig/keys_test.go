// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// Every fault of the bootstrap roots comes in one report, each naming its key
// and file, and none a Go type.
func TestEveryBootstrapFaultIsReportedTogether(t *testing.T) {
	path := writeTemp(t, `version: 1
workspace:
  name: Acme
  base_currency: EURO
  base_language: fr
  timezone: Mars/Base
bootstrap_admin:
  email: admin@acme.test
  dispaly_name: Admin
  password: "${file:/run/secrets/admin}"
seeds:
  starter_automations: maybe
  pipeline:
    name: Sales
    stages:
      - { name: Won, probability: 140 }
`)
	_, err := LoadBootstrap(path)
	if err == nil {
		t.Fatal("a file with six faults loaded")
	}
	msg := err.Error()
	for _, want := range []string{
		path + ": unknown key bootstrap_admin.dispaly_name (line 9); did you mean bootstrap_admin.display_name?",
		"workspace.base_currency in " + path + ` (base_file): "EURO" is not a 3-letter ISO 4217 code in capitals`,
		"workspace.base_language in " + path + ` (base_file): "fr" is not a language this build speaks (en, de, vi)`,
		"workspace.timezone in " + path + " (base_file): Mars/Base is not a known IANA zone",
		"seeds.starter_automations in " + path + " (base_file): want true or false (line 12)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "deployconfig.") || strings.Contains(msg, "in type") {
		t.Errorf("a fault names a Go type:\n%s", msg)
	}
	// The seeds root faulted on its keys, so its values are not checked too.
	if strings.Contains(msg, "probability") {
		t.Errorf("a root with a key fault also reported its values:\n%s", msg)
	}
	// The misspelling is the fault. Reporting the key it meant as missing too
	// would send the operator after a second problem that is the same one.
	if strings.Contains(msg, "display_name in") {
		t.Errorf("the misspelt key also reported the key it meant:\n%s", msg)
	}
}

func TestAKeyOfTheWrongKindIsAFault(t *testing.T) {
	for doc, want := range map[string]string{
		"bootstrap_admin: admin\n":                                 "bootstrap_admin in %s (base_file): want a mapping of keys (line 1)",
		"seeds:\n  consent_purposes: marketing\n":                  "seeds.consent_purposes in %s (base_file): want a list (line 2)",
		"seeds:\n  pipeline:\n    stages: [{probability: high}]\n": "seeds.pipeline.stages[0].probability in %s (base_file): want a whole number (line 3)",
		"seeds:\n  pipeline:\n    stages: [{nam: Won}]\n":          "%s: unknown key seeds.pipeline.stages[0].nam (line 3); did you mean seeds.pipeline.stages[0].name?",
	} {
		path := writeTemp(t, doc)
		_, err := LoadBootstrap(path)
		if want = fmt.Sprintf(want, path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", doc, err, want)
		}
	}
}

// A `<<:` merge brings its keys into the mapping, and a misspelt one among
// them is a fault like any other.
func TestAMergedKeyIsCheckedLikeAWrittenOne(t *testing.T) {
	ok := writeTemp(t, "defaults: &d\n  name: Acme\n  timezone: Europe/Berlin\nworkspace:\n  <<: *d\n  base_currency: EUR\n")
	if boot, err := LoadBootstrap(ok); err != nil || boot.Workspace.Timezone != "Europe/Berlin" {
		t.Fatalf("a merged workspace: boot %+v, err %v", boot.Workspace, err)
	}
	bad := writeTemp(t, "defaults: &d\n  nam: Acme\nworkspace:\n  <<: *d\n")
	if _, err := LoadBootstrap(bad); err == nil || !strings.Contains(err.Error(), "unknown key workspace.nam") {
		t.Fatalf("a misspelt merged key: %v", err)
	}
}

// The boot report leaves out a file it cannot parse, which config.Load
// reports, and keeps the faults of the file it can.
func TestFaultsLeaveAnUnparseableFileToTheSettingsLoader(t *testing.T) {
	base := writeTemp(t, "workspace: [unclosed\n")
	over := writeTemp(t, "workspace:\n  nam: Acme\n")
	faults := Faults(config.Files{Base: base, ConfigFile: over})
	if len(faults) != 1 || !strings.Contains(faults[0].Error(), over+": unknown key workspace.nam (line 2); did you mean workspace.name?") {
		t.Fatalf("faults %v, want only the config file's misspelling", faults)
	}
	if _, err := LoadBootstrapFiles(base, over); err == nil || !strings.Contains(err.Error(), base) {
		t.Fatalf("LoadBootstrapFiles hid the unparseable base: %v", err)
	}
}

// A value fault names the file whose root won, not always the base.
func TestAValueFaultNamesTheFileThatWroteIt(t *testing.T) {
	base := writeTemp(t, "version: 1\n")
	over := writeTemp(t, "workspace:\n  base_currency: eur\n")
	_, err := LoadBootstrapFiles(base, over)
	if err == nil || !strings.Contains(err.Error(), "workspace.base_currency in "+over+" (config_file)") {
		t.Fatalf("got %v", err)
	}
}

// The schema states these rules, so each must refuse what boot refuses and
// take what it takes.
func TestEachValueRuleAgreesWithTheBootCheck(t *testing.T) {
	samples := []string{"EUR", "USD", "EURO", "eur", "E1R", "en", "de", "vi", "fr", "EN", ""}
	rules := ValueRules()
	if len(rules) == 0 {
		t.Fatal("no value rule; this test checks nothing")
	}
	for key, rule := range rules {
		field, ok := strings.CutPrefix(key, "workspace.")
		if !ok {
			t.Fatalf("%s: only workspace rules are sampled here", key)
		}
		for _, value := range append(samples, rule.Enum...) {
			if value == "" {
				continue
			}
			boot := Bootstrap{Version: 1}
			set := map[string]*string{"base_currency": &boot.Workspace.BaseCurrency, "base_language": &boot.Workspace.BaseLanguage}[field]
			if set == nil {
				t.Fatalf("%s: no sample sets this field", key)
			}
			*set = value
			bootTakes := len(boot.validate()) == 0
			ruleTakes := (rule.Enum == nil || slices.Contains(rule.Enum, value)) &&
				(rule.Pattern == "" || regexp.MustCompile(rule.Pattern).MatchString(value))
			if bootTakes != ruleTakes {
				t.Errorf("%s = %q: boot takes it %v, the schema rule %v", key, value, bootTakes, ruleTakes)
			}
		}
	}
}
