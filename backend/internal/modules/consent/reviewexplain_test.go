// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Every refusal is explained by its own fact, and what the sentence offers is
// what the remedy table declares. A stop offers no exception and says nobody
// sends past it; a refusal with an exception says so.
//
// Mutation: hard-code "needs a recorded exception" for the objection again and
// its case fails.
func TestEveryRefusalExplanationFollowsTheRemedyTable(t *testing.T) {
	t.Parallel()
	for _, reason := range commsauthz.RefusalCodes() {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			if _, ok := refusalFacts[reason]; !ok {
				t.Fatalf("%q has no fact of its own, so the rep is shown its code", reason)
			}
			why := whyAStatementCannotAnswer(reason)
			offersException := strings.Contains(why, "exception")
			hasException := commsauthz.HasRemedy(reason, commsauthz.RemedyException) ||
				commsauthz.HasRemedy(reason, commsauthz.RemedyAdminException)
			if offersException != hasException {
				t.Errorf("%q: the text offers an exception = %v, the remedy table declares one = %v: %q",
					reason, offersException, hasException, why)
			}
			if stop := len(commsauthz.RemediesFor(reason)) == 0; stop != strings.Contains(why, "nobody can send past it") {
				t.Errorf("%q: a stop with no remedy must say nobody sends past it, and nothing else may: %q", reason, why)
			}
		})
	}
}

// A stop says what can still go to the contact.
func TestAStopSaysWhatCanStillGo(t *testing.T) {
	t.Parallel()
	for reason, still := range map[string]string{
		commsauthz.ReasonObjection:      "a reply or a message they asked for still can",
		commsauthz.ReasonSubjectRequest: "only security, privacy and opt-out notices still go",
		commsauthz.ReasonRestricted:     "only security, privacy and opt-out notices still go",
	} {
		if why := whyAStatementCannotAnswer(reason); !strings.Contains(why, still) {
			t.Errorf("%q does not say what can still go: %q", reason, why)
		}
	}
}

// A request answering a reply, a follow-up or a quote is read with no age
// limit, so the review form takes it at any age. Anything else keeps the
// window.
func TestOnlyARequestForAClaimItAnswersSkipsTheWindow(t *testing.T) {
	t.Parallel()
	for _, claim := range []commsauthz.Category{
		commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup, commsauthz.CategoryPrecontractQuote,
	} {
		if !requestAnswersClaim(KindRequestedBySubject, claim) {
			t.Errorf("a request answering %s is held to the window", claim)
		}
	}
	if requestAnswersClaim(kindInPerson, commsauthz.CategoryRequestedFollowup) {
		t.Error("an exchange in person skips the window, though no request reader takes it")
	}
	if requestAnswersClaim(KindRequestedBySubject, commsauthz.CategoryActiveDealFollowup) {
		t.Error("a request answering a deal follow-up skips the window, though that validator reads no request")
	}
}
