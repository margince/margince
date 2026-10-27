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
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// approvalSummaryHoles is the verbs each call site passes, in order. It is the
// contract between the table and the Sprintf that reads it, so it is written
// down here rather than derived from the English entry it has to hold.
var approvalSummaryHoles = map[string][]string{
	"signaturePrivate":             nil,
	"draftedReplyWaiting":          {"%q"},
	"counterpartyWorthKeeping":     {"%s"},
	"contactShareAsked":            {"%s"},
	"companyRename":                {"%s", "%s"},
	"siteLeadFound":                {"%s", "%s", "%s"},
	"enrichmentOf":                 {"%s"},
	"contractEndedMove":            {"%s", "%s"},
	"linkedInLooksLike":            {"%s", "%s", "%s"},
	"vcardResemblesContact":        {"%s"},
	"vcardReadFromPhoto":           {"%s"},
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
	"createdUnder":                 nil,
	"nestedFields":                 {"%d"},
}

func TestEveryShippedLanguageWritesItsOwnApprovalSummaries(t *testing.T) {
	langcopytest.CensusByLanguage(t, approvalSummaryByLang, approvalSummaryHoles, "acts", "lang")
}

// The German copy uses the plain hyphen only. A dash belongs to the English
// sentences, which keep theirs byte for byte.
func TestTheGermanApprovalSummariesCarryNoDash(t *testing.T) {
	german := approvalSummaryByLang[textlang.German]
	sentences := langcopytest.Sentences(t, german, "acts", "lang")
	for _, words := range []map[string]string{german.acts.verbs, german.acts.recordFrames, german.acts.operations} {
		maps.Copy(sentences, words)
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
// stageable cannot leave a dead word behind. A registry-served route stages
// through its tool, as the MCP door does, so its method decides nothing here.
func TestEveryStageableAgentActHasItsWordsInEveryLanguage(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			continue
		}
		acts := approvalSummaryByLang[lang].acts
		reachedVerbs, reachedFrames := map[string]bool{}, map[string]bool{}
		for route, pol := range agentPolicies {
			method, _, _ := strings.Cut(route, " ")
			if pol.Access != accessTool || !mutatingMethod(method) || pol.ServedBy == servedByRegistry {
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
