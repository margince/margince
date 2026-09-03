// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a consent link may and may not do.
//
// The link is the whole of the double-opt-in evidence now: the server picked
// the address, the plaintext was mailed and never returned, and spending it is
// what proves the mailbox. These hold the properties that keep that true.

import (
	"context"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A consent link asked one question in the mail it arrived in and may answer
// only that one. A link that could also edit the record or file an erasure
// would be a wider capability than the mail described, and the mail is what the
// subject chose to open.
//
// Asserted through refuseWiderThanTheMail, which is the branch SubmitConfirmation
// takes before it touches anything.
func TestAConsentLinkRefusesWhatTheMailDidNotAskAbout(t *testing.T) {
	cases := map[string]ConfirmSubmission{
		"a correction":       {Corrections: map[string]string{"full_name": "New Name"}},
		"an erasure request": {RequestErasure: true},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if err := refuseWiderThanTheMail(LinkConsentConfirmation, in); err == nil {
				t.Error("a consent link must refuse a submission that changes the record")
			}
		})
	}
}

// And the converse, or the test above would pass with every submission refused:
// the marketing answer the link exists to collect goes through.
func TestAConsentLinkAcceptsTheAnswerItAskedFor(t *testing.T) {
	in := ConfirmSubmission{
		MarketingChoice:  string(StateGranted),
		MarketingWording: "Yes, send me the newsletter.",
	}
	if err := refuseWiderThanTheMail(LinkConsentConfirmation, in); err != nil {
		t.Errorf("refuseWiderThanTheMail = %v, want nil for the answer the link asked for", err)
	}
}

// A record link is unchanged by any of this: it still carries corrections and
// erasure requests, which is what the confirm-details page is for.
func TestARecordLinkStillCarriesCorrections(t *testing.T) {
	in := ConfirmSubmission{
		Corrections:    map[string]string{"full_name": "New Name"},
		RequestErasure: true,
	}
	if err := refuseWiderThanTheMail(LinkRecordConfirmation, in); err != nil {
		t.Errorf("refuseWiderThanTheMail = %v, want nil for a record link", err)
	}
}

// A consent link needs the purpose it confirms. The row's own CHECK says the
// same thing; refusing here names the field instead of surfacing a constraint
// violation, and it happens before any database work.
func TestAConsentLinkRefusesWithoutAPurpose(t *testing.T) {
	_, err := (&Store{}).IssueConsentLink(context.Background(),
		ids.New[ids.PersonKind](), ids.PurposeID{})
	if err == nil {
		t.Fatal("a consent link with no purpose must be refused before it is minted")
	}
}

// The READ carries the same gate as the write.
//
// A consent link's mail asked one question. Serving the record card on the page
// it opens would hand whoever holds that link the person's name, employer,
// address, phone and the whole provenance trail — wider than the mail, and
// wider than the same link's submit is allowed to be. The write side was gated
// first and the read side was not, which is the asymmetry this holds shut.
//
// Asserted against the TYPE rather than a value: consentCardFor's return shape
// is the whole surface a consent link can read, so a field added to it is a
// field the link starts disclosing. reflect is what makes that fail here rather
// than in somebody's inbox.
func TestAConsentLinkReadServesOnlyTheSubscriptionQuestion(t *testing.T) {
	got := map[string]bool{}
	ct := reflect.TypeOf(SubscriptionCard{})
	for i := range ct.NumField() {
		got[ct.Field(i).Name] = true
	}
	want := map[string]bool{"PurposeKey": true, "PurposeLabel": true, "State": true}

	for name := range got {
		if !want[name] {
			t.Errorf("SubscriptionCard carries %q — a consent link may show the subscription it asks about and nothing else", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("SubscriptionCard lost %q, which the page needs to state the question", name)
		}
	}
}

// And the record card is the thing it must not be: if these two ever converge,
// the gate above has stopped meaning anything.
func TestTheSubscriptionCardIsNotTheRecordCard(t *testing.T) {
	consent := reflect.TypeOf(SubscriptionCard{})
	record := reflect.TypeOf(ConfirmCard{})
	if consent.NumField() >= record.NumField() {
		t.Errorf("SubscriptionCard has %d fields and ConfirmCard %d — the consent view must stay the narrower one",
			consent.NumField(), record.NumField())
	}
}
