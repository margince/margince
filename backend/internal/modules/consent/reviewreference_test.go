// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A refused send names each refused address with its own reason, so a sender
// writing to two people learns that one objected and the other lacks a deal
// link, and never reads one cause as everybody's.
func TestARefusalNamesEachRefusedRecipientWithItsOwnReason(t *testing.T) {
	refusal := &SendRefusedError{
		ReviewID: ids.NewV7(),
		Actions:  []string{"overrule"},
		Refusals: []RefusedRecipient{
			{Address: "ana@example.test", ReasonCode: commsauthz.ReasonObjection, Category: "marketing", SubjectID: ids.NewV7().String()},
			{Address: "ben@example.test", ReasonCode: commsauthz.ReasonNoEvidence, Category: "active_deal_followup"},
			{Address: "cy@example.test", ReasonCode: commsauthz.ReasonNoSubject},
		},
	}

	got := refusal.FaultReference()["refusals"]
	want := []map[string]string{
		{"address": "ana@example.test", "reason_code": "marketing_objection", "category": "marketing"},
		{"address": "ben@example.test", "reason_code": "no_compatible_evidence", "category": "active_deal_followup"},
		{"address": "cy@example.test", "reason_code": "recipient_resolves_to_no_single_subject"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refusals on the wire = %#v, want %#v; the subject id must stay off it", got, want)
	}
}

// A refusal that recorded no review carries no reference at all, refusals
// included: the reasons travel with the review they describe.
func TestARefusalWithoutAReviewCarriesNoReasons(t *testing.T) {
	refusal := &SendRefusedError{Refusals: []RefusedRecipient{{Address: "ana@example.test", ReasonCode: commsauthz.ReasonObjection}}}
	if got := refusal.FaultReference(); got != nil {
		t.Errorf("reference without a review = %#v, want nil", got)
	}
}

// A deny the rollout mode only records did not stop the send, so the sender is
// not told about it. An absolute reason stops it under any mode.
func TestOnlyTheRecipientsThatStoppedTheSendAreNamed(t *testing.T) {
	decision := func(email, reason string, verdict commsauthz.Verdict, mode commsauthz.Mode) commsauthz.Decision {
		return commsauthz.Decision{
			Recipient: connector.Recipient{Email: email}, Verdict: verdict,
			ReasonCode: reason, Mode: mode, Resolved: commsauthz.CategoryMarketing,
		}
	}
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		decision("observed@example.test", commsauthz.ReasonNoMarketingConsent, commsauthz.VerdictDeny, commsauthz.ModeObserve),
		decision(" Enforced@Example.test", commsauthz.ReasonNoMarketingConsent, commsauthz.VerdictDeny, commsauthz.ModeEnforce),
		decision("objected@example.test", commsauthz.ReasonObjection, commsauthz.VerdictDeny, commsauthz.ModeObserve),
		decision("allowed@example.test", commsauthz.ReasonAllowed, commsauthz.VerdictAllow, commsauthz.ModeEnforce),
	}}

	var got []string
	for _, r := range BlockingRefusals(set) {
		got = append(got, r.Address)
	}
	want := []string{"enforced@example.test", "objected@example.test"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blocking refusals = %v, want %v; a deny under observe did not stop the send", got, want)
	}
}
