// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// styleCase is one sentence and the rule it must break, or "" for none.
type styleCase struct {
	text, breaks string
}

// rulesReported answers the rule ids a check reported, in report order.
func rulesReported(check func(reporter)) []string {
	s := run(check)
	var ids []string
	for _, e := range s.errors {
		for _, rule := range styleRules {
			if strings.Contains(e, "("+rule.id+",") {
				ids = append(ids, rule.id)
			}
		}
	}
	if s.fatal != "" {
		ids = append(ids, "fatal: "+s.fatal)
	}
	return ids
}

func assertStyle(t *testing.T, cases []styleCase, check func(r reporter, text string)) {
	t.Helper()
	for _, c := range cases {
		got := rulesReported(func(r reporter) { check(r, c.text) })
		switch {
		case c.breaks == "" && len(got) != 0:
			t.Errorf("%q passes every rule, and was reported for %v", c.text, got)
		case c.breaks != "" && (len(got) != 1 || got[0] != c.breaks):
			t.Errorf("%q breaks %s alone, and was reported for %v", c.text, c.breaks, got)
		}
	}
}

func TestEnglishTextHoldsTheMechanicalRules(t *testing.T) {
	t.Parallel()
	assertStyle(t, []styleCase{
		{"Changes were saved — retry later", ruleEnDash},
		{"Changes were saved -- retry later", ruleEnDash},
		{"Changes were saved - retry later", ruleEnDash},
		{"Changes were saved. Retry later.", ""},
		{"A read-only field", ""},
		{"Changes can't be saved", ruleEnContraction},
		{"Changes can’t be saved", ruleEnContraction},
		{"Changes cannot be saved", ""},
		{"We could not save the changes", ruleEnWe},
		{"Sent to our team", ruleEnWe},
		{"English (US)", ""},
		{"I saved the changes", ruleEnI},
		{"Assign to me", ""},
		{"Pick a colour", ruleEnSpelling},
		{"Meeting cancelled", ruleEnSpelling},
		{"Pick a color", ""},
		{"Sales & marketing", ruleEnAmpersand},
		{"Open “R&D” in the other product", ""},
		{"Moved %[1]s to {us}", ""},
	}, func(r reporter, text string) { EnglishText(r, "Field", text) })
}

func TestEachRegisterHoldsItsOwnEnglishVoice(t *testing.T) {
	t.Parallel()
	controller := Controller("the data controller writes to a data subject")
	agent := Agent("an agent speaks in a chat bubble")
	own := OwnVoice("the member sends this as themselves")
	verbatim := Verbatim("a sample cell the reader copies as it stands")
	for _, c := range []struct {
		register Register
		cases    []styleCase
	}{
		{controller, []styleCase{{"We hold your details", ""}, {"I hold your details", ruleEnI}}},
		{agent, []styleCase{{"I read the website", ""}, {"We read the website", ruleEnWe}}},
		{own, []styleCase{{"I'll send what we owe", ""}, {"Thanks — speak soon", ruleEnDash}}},
		{verbatim, []styleCase{{"Acme GmbH & Co. KG — we", ""}}},
	} {
		assertStyle(t, c.cases, func(r reporter, text string) { c.register.EnglishText(r, "Field", text) })
	}
}

func TestCensusHoldsEveryPhraseToTheStylePages(t *testing.T) {
	t.Parallel()
	type table struct{ Saved langcopy.Phrase }
	bad := table{Saved: langcopy.Phrase{En: "We saved it", De: "Gespeichert", Vi: "Chúng tôi đã lưu"}}
	got := rulesReported(func(r reporter) { Census(r, bad) })
	if strings.Join(got, " ") != ruleEnWe+" "+ruleViChungToi {
		t.Errorf("a phrase speaking as we in English and Vietnamese was reported for %v", got)
	}
	got = rulesReported(func(r reporter) { Census(r, bad, Controller("the data controller writes this")) })
	if len(got) != 0 {
		t.Errorf("the controller's we was reported for %v", got)
	}
}

func TestCensusNamesOneRegisterAndWhy(t *testing.T) {
	t.Parallel()
	type table struct{ Saved langcopy.Phrase }
	good := table{Saved: langcopy.Phrase{En: "Saved", De: "Gespeichert", Vi: "Đã lưu"}}
	for name, registers := range map[string][]Register{
		"no reason": {Outsider("  ")},
		"two":       {Outsider("a buyer reads it"), Agent("an agent says it")},
		"zero":      {{}},
	} {
		if s := run(func(r reporter) { Census(r, good, registers...) }); s.fatal == "" {
			t.Errorf("%s: a census with registers %v was not refused", name, registers)
		}
	}
}

func TestEveryRegisterNamesRulesThatExist(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, rule := range styleRules {
		if seen[rule.id] {
			t.Errorf("%s names two rules", rule.id)
		}
		seen[rule.id] = true
	}
	for _, register := range []Register{member, Agent("x"), Outsider("x"), Controller("x"), OwnVoice("x"), Verbatim("x")} {
		for _, id := range append(append([]string{}, register.drops...), register.keeps...) {
			if !seen[id] {
				t.Errorf("the %s register names %s, which is no rule", register.name, id)
			}
		}
	}
}

func TestCensusByLanguageHoldsTheStylePages(t *testing.T) {
	t.Parallel()
	type summary struct{ sentence string }
	table := map[textlang.Lang]summary{
		textlang.English: {"We wrote last"}, textlang.German: {"Zuletzt geschrieben"}, textlang.Vietnamese: {"Đã viết lần cuối"},
	}
	got := rulesReported(func(r reporter) { CensusByLanguage(r, table, map[string][]string{"sentence": nil}) })
	if strings.Join(got, " ") != ruleEnWe {
		t.Errorf("a legacy table speaking as we was reported for %v", got)
	}
}
