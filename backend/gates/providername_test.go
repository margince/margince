// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The rule a provider name must satisfy is the CONTRACT's, spelled once.
//
// `Provider` is published as a pattern-constrained string rather than an enum,
// because which providers exist is a deployment fact — what this binary
// composed — and an enum would assert the legal set is identical everywhere.
// A pattern in a schema is not enforcement, though: the registry admitted any
// non-empty unique name, so an adapter called `Acme-Data` would register, write
// rows, and be serialized into a response that violates the published schema.
//
// The registry checks it now, against a Go spelling of the same pattern. Two
// spellings of one rule drift, and this drift would be silent in the direction
// that matters: a Go pattern LOOSER than the contract admits names the schema
// refuses, which is the defect the check exists to remove, and it would be
// invisible until a client validated a response.
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

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// providerNameContract is where the published rule lives.
const providerNameContract = "api/crm.yaml"

// TestTheProviderNameRuleIsTheContractsOwn is the parity check.
func TestTheProviderNameRuleIsTheContractsOwn(t *testing.T) {
	t.Parallel()
	pattern, maxLength := publishedProviderNameRule(t)
	if pattern != provider.NamePattern {
		t.Errorf("the contract publishes provider names as %s and the registry admits %s — a Go pattern looser than "+
			"the contract registers an adapter whose name the schema refuses, and nothing says so until a client "+
			"validates a response",
			pattern, provider.NamePattern)
	}
	if maxLength != provider.NameMaxLength {
		t.Errorf("the contract caps a provider name at %d characters and the registry at %d",
			maxLength, provider.NameMaxLength)
	}
}

// publishedProviderNameRule reads the Provider schema's own constraints.
func publishedProviderNameRule(t *testing.T) (pattern string, maxLength int) {
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
	schema, declared := contract.Components.Schemas["Provider"]
	if !declared {
		t.Fatal("the contract declares no Provider schema: this gate compares the registry's name rule against " +
			"that schema's own, so it now certifies nothing")
	}
	if schema.Pattern == "" || schema.MaxLength == 0 {
		t.Fatalf("the Provider schema declares pattern %q and maxLength %d — a rule this gate cannot read is one "+
			"it cannot hold the registry to, and an empty comparison passes",
			schema.Pattern, schema.MaxLength)
	}
	return schema.Pattern, schema.MaxLength
}

// TestTheProviderNameRuleAdmitsWhatTheContractDoes is the rule itself,
// exercised on the shapes an extension author actually reaches for.
func TestTheProviderNameRuleAdmitsWhatTheContractDoes(t *testing.T) {
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
				t.Errorf("ValidName(%q) = %v, want %v — %s", c.name, got, c.admits, c.because)
			}
		})
	}
}
