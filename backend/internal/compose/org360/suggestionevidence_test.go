// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A citation is the reader's receipt. Beside the record's id it carries the
// record's own words, the date they are dated, and where they came from — so
// the advice is checked against the evidence at a glance rather than trusted
// or opened. None of it reaches the fingerprint, because none of it changes
// what the rule fired ON.

func TestTheNoReplyCitationCarriesTheMessagesOwnWords(t *testing.T) {
	newest := sentAgo(10, crmcontracts.ActivityDirectionOutbound)
	newest.Kind = string(crmcontracts.ActivityKindEmail)
	newest.Subject = "Slots for the pilot review"
	newest.Excerpt = "Hi Anna, two slots next week would work on our side."

	got := staleThread(testOrgID(t), suggestNow, newest)
	if got == nil {
		t.Fatal("the rule did not fire on a 10-day-old unanswered email")
	}
	cited := got.Evidence[0]
	if cited.Name == nil || *cited.Name != newest.Subject {
		t.Errorf("name = %v, want the subject line the chip is labelled with", cited.Name)
	}
	if cited.Quote == nil || *cited.Quote != newest.Excerpt {
		t.Errorf("quote = %v, want the message's opening words", cited.Quote)
	}
	if cited.At == nil || !cited.At.Equal(newest.At) {
		t.Errorf("at = %v, want the instant the message was sent", cited.At)
	}
	if cited.Origin == nil || *cited.Origin != "Email you sent" {
		t.Errorf("origin = %v, want the channel we spoke on", cited.Origin)
	}
}

// A limited audience withholds the words and not the fact: the reader is
// still told they are waiting on a reply, and the chip carries no quote it
// would have to invent.
func TestTheNoReplyCitationWithholdsWordsTheReaderMayNotRead(t *testing.T) {
	newest := sentAgo(10, crmcontracts.ActivityDirectionOutbound)
	newest.Kind = string(crmcontracts.ActivityKindCall)

	got := staleThread(testOrgID(t), suggestNow, newest)
	if got == nil {
		t.Fatal("the rule went silent on a message whose words are withheld")
	}
	cited := got.Evidence[0]
	if cited.Name != nil || cited.Quote != nil {
		t.Errorf("name = %v, quote = %v; a withheld message must carry no words at all", cited.Name, cited.Quote)
	}
	if cited.At == nil || cited.Origin == nil || *cited.Origin != "Call you made" {
		t.Errorf("at = %v, origin = %v; the date and the channel are the rule's own facts", cited.At, cited.Origin)
	}
}

func TestTheStalledDealCitationSaysWhenItWasLastWorked(t *testing.T) {
	deal := idle("Renewal")

	got := stalledDealSuggestions([]stalledDeal{deal})
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want one", len(got))
	}
	cited := got[0].Evidence[0]
	if cited.Name == nil || *cited.Name != "Renewal" {
		t.Errorf("name = %v, want the deal's own name", cited.Name)
	}
	if cited.At == nil || !cited.At.Equal(deal.IdleSince) {
		t.Errorf("at = %v, want the instant the deal was last worked", cited.At)
	}
	if cited.Origin == nil || *cited.Origin != originStalledDeal {
		t.Errorf("origin = %v, want %q", cited.Origin, originStalledDeal)
	}
}

func TestTheConflictQuotesTheMailThatEndedTheContract(t *testing.T) {
	read := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	in := suggestionInputs{
		contractEnded:     true,
		contractEndedSaid: "We will not be renewing after July.",
		contractEndedAt:   read,
		lifecycle:         "customer",
	}

	got := lifecycleConflict(testOrgID(t), in)
	if got == nil {
		t.Fatal("no conflict raised on a live customer whose mail ended the contract")
	}
	cited := got.Evidence[0]
	if cited.Quote == nil || *cited.Quote != in.contractEndedSaid {
		t.Errorf("quote = %v, want the signal's own sentence", cited.Quote)
	}
	if cited.At == nil || !cited.At.Equal(read) {
		t.Errorf("at = %v, want when the signal was read", cited.At)
	}
	if cited.Origin == nil || *cited.Origin != originContractEnded {
		t.Errorf("origin = %v, want %q", cited.Origin, originContractEnded)
	}
}

// The words decorate the receipt; the fingerprint identifies the situation. A
// subject edited after the fact, or a body the reader could not see yesterday
// and can today, must not resurrect a dismissal.
func TestTheCitationsWordsNeverReachTheFingerprint(t *testing.T) {
	orgID := testOrgID(t)
	bare := sentAgo(10, crmcontracts.ActivityDirectionOutbound)
	worded := bare
	worded.Kind, worded.Subject, worded.Excerpt = "email", "Re: pilot", "Two slots next week."

	if a, b := staleThread(orgID, suggestNow, bare), staleThread(orgID, suggestNow, worded); a.Fingerprint != b.Fingerprint {
		t.Errorf("the no-reply fingerprint moved with the words:\n  %s\n  %s", a.Fingerprint, b.Fingerprint)
	}

	dealID := ids.NewV7()
	unnamed := stalledDeal{ID: dealID, Name: "Renewal", IdleSince: suggestNow.AddDate(0, 0, -90)}
	renamed := stalledDeal{ID: dealID, Name: "Renewal 2027", IdleSince: unnamed.IdleSince}
	a, b := stalledDealSuggestions([]stalledDeal{unnamed}), stalledDealSuggestions([]stalledDeal{renamed})
	if a[0].Fingerprint != b[0].Fingerprint {
		t.Errorf("the stalled-deal fingerprint moved with the name:\n  %s\n  %s", a[0].Fingerprint, b[0].Fingerprint)
	}
}
