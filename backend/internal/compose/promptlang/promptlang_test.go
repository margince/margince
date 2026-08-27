// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package promptlang

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Rule names the language in English, and every shipped language must have a
// name. A language the setting accepts but this package cannot name would fall
// back to English silently — the installation would set German, the setting
// would take it, and every brief would come back in English with nothing
// anywhere saying why.
//
// Driven off textlang.Shipped rather than a written-out list so that adding a
// fourth language fails HERE, naming the language nobody wired, rather than in
// production.
func TestEveryShippedLanguageHasAName(t *testing.T) {
	for _, lang := range textlang.Shipped {
		rule := Rule(string(lang))
		if !strings.HasPrefix(rule, Heading) {
			t.Errorf("the rule for %q does not open with the heading the gate recognises", lang)
		}
		if englishName(string(lang)) == "" {
			t.Errorf("language %q is shipped but has no English name here, so its prompts would silently be written in English", lang)
		}
	}
}

// Two different languages must produce two different rules. A Rule that
// returned the same text for every code would satisfy every "does the prompt
// carry a rule" check in the tree while instructing nothing — the failure this
// package exists to prevent, wearing the shape of the fix.
func TestTwoLanguagesProduceDifferentRules(t *testing.T) {
	seen := map[string]string{}
	for _, lang := range textlang.Shipped {
		rule := Rule(string(lang))
		if other, clash := seen[rule]; clash {
			t.Fatalf("languages %q and %q produce identical rules, so neither instructs anything", other, lang)
		}
		seen[rule] = string(lang)
	}
}

// A code the product does not ship falls back to English rather than refusing
// or interpolating the code itself. Refusing would take a whole brief down over
// a settings row somebody edited by hand; interpolating would instruct the
// model in a language nobody named.
func TestAnUnknownLanguageFallsBackToEnglish(t *testing.T) {
	for _, unknown := range []string{"", "xx", "klingon", "de-CH", "EN"} {
		got := Rule(unknown)
		if got != Rule(string(textlang.English)) {
			t.Errorf("Rule(%q) did not fall back to the English rule; got:\n%s", unknown, got)
		}
		if strings.Contains(got, unknown) && unknown != "" {
			t.Errorf("Rule(%q) interpolated the unrecognised code into the prompt, instructing the model in a language nobody named", unknown)
		}
	}
}

// The carve-outs are the half that keeps the rule from breaking things. A model
// told to write in German will otherwise translate a status value or a JSON
// key, and a translated enum is a parse failure rather than a style problem.
func TestTheRuleProtectsWhatMustNotBeTranslated(t *testing.T) {
	rule := Rule(string(textlang.German))
	for _, protected := range []string{"JSON keys", "enum", "ids", "names", "quoting"} {
		if !strings.Contains(rule, protected) {
			t.Errorf("the rule does not tell the model to leave %s alone:\n%s", protected, rule)
		}
	}
}
