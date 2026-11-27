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
// suite and, against this list, by localedoc_test.go; this holds the two halves
// a Go change can break.

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The enums that must list exactly the shipped languages, by the schema they
// belong to. Each is a place a client learns which languages it may send.
var languageEnumSchemas = []string{
	"InstallationSettings",
	"UpdateInstallationSettingsRequest",
	"SaveMyLocaleRequest",
	"User",
	// A captured message's own detected language. Nullable, so its enum carries
	// `null` beside the codes — which is not a language and is dropped before
	// the comparison below.
	"Activity",
	"DraftSteering",
	"EmailDraft",
	"CompanyEmailDraft",
	"MailDraftEditorState",
}

// languageEnumVarnames name the onboarding enums by the x-enum-varnames prefix
// each declares, so a contract edit that moves one cannot take it out of view.
var languageEnumVarnames = []string{"OnboardingProposalLocale", "OnboardingCompanyLocale"}

// shippedLanguageSet is textlang.Shipped as sorted codes, which every language
// enum must equal as a set.
func shippedLanguageSet() []string {
	want := make([]string, 0, len(textlang.Shipped))
	for _, lang := range textlang.Shipped {
		want = append(want, string(lang))
	}
	sort.Strings(want)
	return want
}

func TestEveryLanguageEnumInTheContractListsTheShippedLanguages(t *testing.T) {
	t.Parallel()
	contract, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	want := shippedLanguageSet()
	for _, prefix := range languageEnumVarnames {
		named := regexp.MustCompile(`enum: \[([^\]]*)\]\n[^\n]*\n?[^\n]*x-enum-varnames: \[` + prefix).
			FindSubmatch(contract)
		if named == nil {
			t.Errorf("no language enum declares x-enum-varnames %s: if it was renamed, rename it here too", prefix)
			continue
		}
		got := languageMembers(string(named[1]))
		sort.Strings(got)
		if !slices.Equal(got, want) {
			t.Errorf("the %s enum admits %v, but the product ships %v: the frontend maps a refused language "+
				"to English, so the reader is onboarded in the wrong language", prefix, got, want)
		}
	}

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
	property := regexp.MustCompile(`(?m)^\s+(locale|base_language|language):\s*$`)
	loc := property.FindStringIndex(body)
	if loc == nil {
		return nil, false
	}
	enum := regexp.MustCompile(`enum:\s*\[([^\]]*)\]`).FindStringSubmatch(body[loc[1]:])
	if enum == nil {
		return nil, false
	}
	out := languageMembers(enum[1])
	return out, len(out) > 0
}

// languageEnum matches a flow enum. One holding a shipped code is a language
// enum, because no other enum in this contract names `en`, `de` or `vi`. So a
// new surface that takes a language is held without a line above.
var languageEnum = regexp.MustCompile(`enum:\s*\[([^\]]*)\]`)

// TestEveryEnumNamingAShippedLanguageListsThemAll is the census half. The
// named schemas above prove their enums exist; this proves no other language
// enum drifts from the shipped set.
func TestEveryEnumNamingAShippedLanguageListsThemAll(t *testing.T) {
	t.Parallel()
	contract, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	want := strings.Join(shippedLanguageSet(), ",")
	found := 0
	for _, at := range languageEnum.FindAllSubmatchIndex(contract, -1) {
		members := languageMembers(string(contract[at[2]:at[3]]))
		if !slices.ContainsFunc(members, textlang.Known) {
			continue
		}
		found++
		sort.Strings(members)
		if got := strings.Join(members, ","); got != want {
			line := 1 + strings.Count(string(contract[:at[0]]), "\n")
			t.Errorf("api/crm.yaml:%d admits languages [%s], but the product ships [%s]: a code the server "+
				"accepts and the contract refuses is a 422 nobody can explain", line, got, want)
		}
	}
	if floor := len(languageEnumSchemas) + len(languageEnumVarnames); found < floor {
		t.Errorf("found %d enum(s) naming a shipped language, fewer than the %d named above: the reader has stopped matching",
			found, floor)
	}
}

// languageMembers reads a flow enum's members, dropping `null`: a nullable enum
// says the value may be absent, which names no language.
func languageMembers(raw string) []string {
	var out []string
	for value := range strings.SplitSeq(raw, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "null" {
			out = append(out, trimmed)
		}
	}
	return out
}
