// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The rule a REGISTERED NAME must satisfy is the contract's, on both surfaces
// that have one.
//
// `Provider` (a licensed data vendor) and `ProviderRef` (a messaging
// transport) are both published as pattern-constrained strings rather than
// enums, because which of either exists is a deployment fact — what this binary
// composed — and an enum would assert the legal set is identical everywhere. A
// pattern in a schema is not enforcement, though: each registry admitted any
// non-empty unique name, so one called `Acme-Data` would register, write rows,
// and be serialized into a response that violates the published schema.
//
// Each registry checks its own now, against a Go spelling of the pattern. Two
// spellings of one rule drift, and this drift would be silent in the direction
// that matters: a Go pattern LOOSER than the contract admits names the schema
// refuses, which is the defect the check exists to remove, and it would be
// invisible until a client validated a response.
//
// TWO RULES, not one shared constant, because they are two schemas. The shapes
// are identical today and a transport is not a data vendor: one of them can
// gain a character the other must not, and a single constant would carry that
// change to a surface nobody argued it for. What this gate holds is that each
// matches ITS OWN schema.
//
// So the two are compared as text. The contract is parsed rather than
// pattern-matched: rewriting the schema block-style changes nothing about it,
// and a regex over the raw file would walk onto another schema's constraints
// and report a confident wrong answer about a rule it was no longer reading.

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// providerNameContract is where the published rule lives.
const providerNameContract = "api/crm.yaml"

// TestEveryRegisteredNameRuleIsTheContractsOwn is the parity check.
func TestEveryRegisteredNameRuleIsTheContractsOwn(t *testing.T) {
	t.Parallel()
	for _, rule := range []struct {
		schema    string
		what      string
		pattern   string
		maxLength int
	}{
		{schema: "Provider", what: "a licensed data provider", pattern: provider.NamePattern, maxLength: provider.NameMaxLength},
		{schema: "ProviderRef", what: "a messaging transport", pattern: connector.NamePattern, maxLength: connector.NameMaxLength},
	} {
		t.Run(rule.schema, func(t *testing.T) {
			t.Parallel()
			pattern, maxLength := publishedNameRule(t, rule.schema)
			if pattern != rule.pattern {
				t.Errorf("the contract publishes %s as %s and its registry admits %s — a Go pattern looser than "+
					"the contract registers a name the schema refuses, and nothing says so until a client "+
					"validates a response",
					rule.what, pattern, rule.pattern)
			}
			if maxLength != rule.maxLength {
				t.Errorf("the contract caps the name of %s at %d characters and its registry at %d",
					rule.what, maxLength, rule.maxLength)
			}
		})
	}
}

// publishedNameRule reads one schema's own constraints.
func publishedNameRule(t *testing.T, schemaName string) (pattern string, maxLength int) {
	t.Helper()
	document, err := os.ReadFile(providerNameContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var contract struct {
		Components struct {
			// Type is `any`: schemas elsewhere in this document declare
			// theirs as a LIST, and a reader typed to a string fails on
			// the whole file rather than on the one schema it is about.
			Schemas map[string]struct {
				Type    any    `yaml:"type"`
				Pattern string `yaml:"pattern"`
				//nolint:tagliatelle // The key is OpenAPI's, and this reads the document as written.
				MaxLength int `yaml:"maxLength"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(document, &contract); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	schema, declared := contract.Components.Schemas[schemaName]
	if !declared {
		t.Fatalf("the contract declares no %s schema: this gate compares a registry's name rule against that "+
			"schema's own, so it now certifies nothing", schemaName)
	}
	if schema.Pattern == "" || schema.MaxLength == 0 {
		t.Fatalf("the %s schema declares pattern %q and maxLength %d — a rule this gate cannot read is one it "+
			"cannot hold a registry to, and an empty comparison passes",
			schemaName, schema.Pattern, schema.MaxLength)
	}
	return schema.Pattern, schema.MaxLength
}

// TestARegisteredNameRuleAdmitsWhatTheContractDoes is the rule itself,
// exercised on the shapes an extension author actually reaches for. Both
// spellings are checked, so a change to one is not assumed of the other.
func TestARegisteredNameRuleAdmitsWhatTheContractDoes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		admits  bool
		because string
	}{
		{name: "surfe", admits: true, because: "the shape every name has today"},
		{name: "acme_data", admits: true, because: "an underscore is how a two-word vendor is spelled"},
		{name: "vendor2", admits: true, because: "a digit after the first character"},
		{name: "Acme-Data", admits: false, because: "a capital and a hyphen, the shape the issue names"},
		{name: "ACME", admits: false, because: "capitals"},
		{name: "2vendor", admits: false, because: "a leading digit"},
		{name: "acme data", admits: false, because: "a space"},
		{name: "acme.data", admits: false, because: "a dot"},
		{name: "", admits: false, because: "no name at all"},
		{name: strings.Repeat("a", provider.NameMaxLength), admits: true, because: "exactly the ceiling"},
		{name: strings.Repeat("a", provider.NameMaxLength+1), admits: false, because: "one over the ceiling"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := provider.ValidName(c.name); got != c.admits {
				t.Errorf("provider.ValidName(%q) = %v, want %v — %s", c.name, got, c.admits, c.because)
			}
			if got := connector.ValidName(c.name); got != c.admits {
				t.Errorf("connector.ValidName(%q) = %v, want %v — %s", c.name, got, c.admits, c.because)
			}
		})
	}
}
