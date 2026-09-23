// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every shipped language writes an approval card in ITS OWN words, with every
// hole its call site fills.
//
// A census that only checked for an entry would pass against a set copied from
// English and never translated. So each sentence is asserted different from
// English, and its formatting verbs asserted to be exactly the ones the call
// site passes, in the order it passes them: Go's verbs are positional, so a
// translation that drops one prints "%!s(MISSING)" into a shared record, and one
// that reorders them names the wrong thing without any error at all.

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// approvalSummaryHoles is the verbs each call site passes, in order. It is the
// contract between the table and the Sprintf that reads it, so it is written
// down here rather than derived from the English entry it has to hold.
var approvalSummaryHoles = map[string][]string{
	"draftedReplyWaiting":          {"%q"},
	"counterpartyWorthKeeping":     {"%s"},
	"companyRename":                {"%s", "%s"},
	"siteLeadFound":                {"%s", "%s", "%s"},
	"enrichmentOf":                 {"%s"},
	"contractEndedMove":            {"%s", "%s"},
	"overwriteHumanEdited":         {"%s", "%s"},
	"linkedInLooksLike":            {"%s", "%s", "%s"},
	"vcardResemblesContact":        {"%s"},
	"unnamedEmployer":              nil,
	"heldNotSent":                  {"%q", "%s"},
	"heldConsentWithdrawn":         nil,
	"heldSenderInactive":           nil,
	"heldPassportRevoked":          nil,
	"heldMissedWindow":             nil,
	"heldTimerExhausted":           nil,
	"heldSendRefused":              nil,
	"reviewSendRefused":            {"%s", "%s"},
	"reviewOneRecipient":           nil,
	"reviewRecipients":             {"%d"},
	"coldStartFromURL":             {"%s"},
	"coldStartFromText":            nil,
	"coldStartFromSelfDescription": nil,
	"fxRateChanged":                {"%s", "%s", "%s", "%s"},
	"fxNoRateInForce":              nil,
	"modelRateChanged":             {"%s", "%s", "%s", "%s"},
	"modelRateNew":                 nil,
	"createdUnder":                 nil,
	"nestedFields":                 {"%d"},
}

// formatVerb matches a verb with or without an explicit argument index, so an
// index reads as a different verb and fails the comparison.
var formatVerb = regexp.MustCompile(`%(\[\d+\])?[a-zA-Z]`)

// summarySentences reads every string field off one set by name. Walking the
// struct rather than listing it is what puts a new field under the census the
// moment it exists.
func summarySentences(said approvalSummaryCopy) map[string]string {
	value := reflect.ValueOf(said)
	sentences := map[string]string{}
	for i := range value.NumField() {
		// Exactly string: the set's own textlang.Lang is a string kind and no sentence.
		if field := value.Field(i); field.Type() == reflect.TypeFor[string]() {
			sentences[value.Type().Field(i).Name] = field.String()
		}
	}
	return sentences
}

func TestEveryShippedLanguageWritesItsOwnApprovalSummaries(t *testing.T) {
	english := summarySentences(approvalSummaryByLang[textlang.English])
	for name := range english {
		if _, known := approvalSummaryHoles[name]; !known {
			t.Errorf("%s has no entry in approvalSummaryHoles, so nothing holds its call site to its holes", name)
		}
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			set, ok := approvalSummaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for name, got := range summarySentences(set) {
				if got == "" {
					t.Errorf("%s is empty — a blank summary reaches the decider as nothing at all", name)
					continue
				}
				if verbs := formatVerb.FindAllString(got, -1); !slices.Equal(verbs, approvalSummaryHoles[name]) {
					t.Errorf("%s carries the verbs %v, its call site passes %v: %q",
						name, verbs, approvalSummaryHoles[name], got)
				}
				if lang != textlang.English && got == english[name] {
					t.Errorf("%s is the English sentence verbatim; the entry exists and the reader still gets English", name)
				}
			}
		})
	}
}

// The German copy uses the plain hyphen only. A dash belongs to the English
// sentences, which keep theirs byte for byte.
func TestTheGermanApprovalSummariesCarryNoDash(t *testing.T) {
	german := approvalSummaryByLang[textlang.German]
	sentences := summarySentences(german)
	for _, words := range []map[string]string{german.acts.verbs, german.acts.recordFrames, german.acts.operations} {
		for key, phrase := range words {
			sentences[key] = phrase
		}
	}
	for name, sentence := range sentences {
		if strings.ContainsAny(sentence, "—–") {
			t.Errorf("German %s carries a long dash: %q", name, sentence)
		}
	}
}

// Every act an agent's call can be staged for has a word in every language but
// English, whose words are the wire verbs themselves.
//
// The corpus is the policy table's mutating tool routes, not a list: a contract
// change that makes a new verb stageable fails here rather than printing the
// English verb on a German card. It runs both ways, so a verb that stops being
// stageable cannot leave a dead word behind.
func TestEveryStageableAgentActHasItsWordsInEveryLanguage(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			continue
		}
		acts := approvalSummaryByLang[lang].acts
		reachedVerbs, reachedFrames := map[string]bool{}, map[string]bool{}
		for route, pol := range agentPolicies {
			method, _, _ := strings.Cut(route, " ")
			if pol.Access != accessTool || !mutatingMethod(method) {
				continue
			}
			if pol.RecordType == "" || !genericVerbs[pol.Tool] {
				reachedVerbs[pol.Tool] = true
				if acts.verbs[pol.Tool] == "" {
					t.Errorf("%s has no words for %s (%s)", lang, pol.Tool, route)
				}
				continue
			}
			reachedFrames[pol.Tool] = true
			if frame := acts.recordFrames[pol.Tool]; strings.Count(frame, "%s") != 1 {
				t.Errorf("%s's frame for %s must hold exactly one record noun: %q", lang, pol.Tool, frame)
			}
		}
		for op := range opPhrases {
			if acts.operations[op] == "" {
				t.Errorf("%s does not name %s, so its headline collapses into its sibling's", lang, op)
			}
		}
		assertEveryWordIsReached(t, lang, "verb", acts.verbs, reachedVerbs)
		assertEveryWordIsReached(t, lang, "frame", acts.recordFrames, reachedFrames)
		namedOps := map[string]bool{}
		for op := range opPhrases {
			namedOps[op] = true
		}
		assertEveryWordIsReached(t, lang, "operation", acts.operations, namedOps)
	}
}

// assertEveryWordIsReached fails a vocabulary entry no stageable call can name.
func assertEveryWordIsReached[K comparable](t *testing.T, lang textlang.Lang, kind string, words map[K]string, reached map[K]bool) {
	t.Helper()
	for key := range words {
		if reached[key] {
			continue
		}
		t.Errorf("%s carries a %s for %v, which no stageable call names any more", lang, kind, key)
	}
}

// A German card names the act and keeps the call's own fields verbatim.
func TestAGermanCardNamesTheActInGerman(t *testing.T) {
	pol := agentPolicy{Op: "updateDeal", Tool: toolUpdateRecord, RecordType: recordTypeDeal}
	got := restSummary(approvalSummaryCopyFor(textlang.German), pol,
		summaryRequest("PATCH", "/v1/deals/x"), []byte(`{"amount_minor":100}`))
	if want := "Deal ändern: amount_minor=100"; got != want {
		t.Errorf("the German card reads %q, want %q", got, want)
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageWritesEnglishApprovalSummaries(t *testing.T) {
	said := approvalSummaryCopyFor(textlang.Lang("kl"))
	if said.companyRename != approvalSummaryByLang[textlang.English].companyRename {
		t.Errorf("an unshipped language answered %q, want the English sentence", said.companyRename)
	}
}
