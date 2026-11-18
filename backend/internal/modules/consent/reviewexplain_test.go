// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

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

func TestAStopExplainsThatTheRecordedPreferenceRemains(t *testing.T) {
	t.Parallel()
	for reason, unchanged := range map[string]string{
		commsauthz.ReasonObjection:        "does not lift their objection",
		commsauthz.ReasonSubjectRequest:   "does not change their request",
		commsauthz.ReasonRestricted:       "does not lift it",
		commsauthz.ReasonConsentWithdrawn: "does not restore consent",
	} {
		if why := whyAStatementCannotAnswer(reason); !strings.Contains(why, unchanged) {
			t.Errorf("%q: %q", reason, why)
		}
	}
}

// A request answering a reply, a follow-up, a quote or a deal follow-up has no
// age limit. So the review form takes it at any age. Anything else
// keeps the window.
func TestOnlyARequestForAClaimItAnswersSkipsTheWindow(t *testing.T) {
	t.Parallel()
	for _, claim := range []commsauthz.Category{
		commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup, commsauthz.CategoryPrecontractQuote,
		commsauthz.CategoryActiveDealFollowup,
	} {
		if !requestAnswersClaim(KindRequestedBySubject, claim) {
			t.Errorf("a request answering %s is held to the window", claim)
		}
	}
	if requestAnswersClaim(kindInPerson, commsauthz.CategoryRequestedFollowup) {
		t.Error("an exchange in person skips the window, though no request reader takes it")
	}
	if requestAnswersClaim(KindRequestedBySubject, commsauthz.CategoryInvoiceOrPayment) {
		t.Error("a request answering an invoice skips the window, though that validator reads no request")
	}
}
