// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The languages the product speaks are declared in more than one place, and
// they have to agree.
//
// `textlang.Shipped` is what the Go validators answer to: the installation's
// base language and a member's own display language both refuse a code that is
// not on it. The contract declares the same set again as enums, because a
// generated client needs them, and the frontend declares it a third time as
// LOCALES because it needs a catalog per language.
//
// Nothing makes those declarations one declaration. What this test does is make
// them fail together: add a language to the Go list without widening the
// contract and the enum stops admitting a value the server now accepts, which
// is a 422 nobody can explain. The frontend's third copy is checked by its own
// suite; this holds the two halves a Go change can break.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The enums that must list exactly the shipped languages, by the schema they
// belong to. Each is a place a client learns which languages it may send.
var languageEnumSchemas = []string{
	"InstallationSettings",
	"UpdateInstallationSettingsRequest",
	"SaveMyLocaleRequest",
	"User",
}

func TestEveryLanguageEnumInTheContractListsTheShippedLanguages(t *testing.T) {
	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	want := make([]string, 0, len(textlang.Shipped))
	for _, lang := range textlang.Shipped {
		want = append(want, string(lang))
	}
	sort.Strings(want)

	for _, schema := range languageEnumSchemas {
		body, ok := schemaBody(string(contract), schema)
		if !ok {
			t.Errorf("schema %s is not in the contract — if it was renamed, rename it here too", schema)
			continue
		}
		got, ok := languageEnumOf(body)
		if !ok {
			t.Errorf("schema %s declares no language enum; every surface that takes a language must say which ones it takes", schema)
			continue
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("schema %s admits languages %v, but the product ships %v — a code the server accepts and the contract refuses is a 422 nobody can explain",
				schema, got, want)
		}
	}
}

// schemaBody returns the lines of one component schema, which is where its
// properties and their enums live.
func schemaBody(contract, name string) (string, bool) {
	start := strings.Index(contract, "\n    "+name+":\n")
	if start < 0 {
		return "", false
	}
	rest := contract[start+1:]
	// The next sibling schema starts at the same indentation. Everything before
	// it belongs to this one.
	if next := regexp.MustCompile(`\n    [A-Za-z][A-Za-z0-9]*:\n`).FindStringIndex(rest[1:]); next != nil {
		return rest[:next[0]+1], true
	}
	return rest, true
}

// languageEnumOf finds the enum on a `locale` or `base_language` property. The
// property name is what marks it as a language rather than any other enum the
// schema happens to carry.
func languageEnumOf(body string) ([]string, bool) {
	property := regexp.MustCompile(`(?m)^\s+(locale|base_language):\s*$`)
	loc := property.FindStringIndex(body)
	if loc == nil {
		return nil, false
	}
	enum := regexp.MustCompile(`enum:\s*\[([^\]]*)\]`).FindStringSubmatch(body[loc[1]:])
	if enum == nil {
		return nil, false
	}
	var out []string
	for _, value := range strings.Split(enum[1], ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, len(out) > 0
}
