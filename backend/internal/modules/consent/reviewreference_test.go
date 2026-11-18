// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A refused send names each refused address with its own reason. A sender
// writing to two recipients learns that one objected and the other lacks a
// deal link.
func TestARefusalNamesEachRefusedRecipientWithItsOwnReason(t *testing.T) {
	refusal := &SendRefusedError{
		ReviewID: ids.NewV7(),
		Actions:  []string{"direct_send"},
		Refusals: []RefusedRecipient{
			{Address: "ana@example.test", ReasonCode: commsauthz.ReasonObjection, Category: "marketing", SubjectID: ids.NewV7().String()},
			{Address: "ben@example.test", ReasonCode: commsauthz.ReasonNoEvidence, Category: "requested_followup", RequestedCategory: "active_deal_followup", CanBeOverruled: true},
			{Address: "cy@example.test", ReasonCode: commsauthz.ReasonNoSubject},
		},
	}

	got := refusal.FaultReference()["refusals"]
	want := []map[string]any{
		{"address": "ana@example.test", "reason_code": "marketing_objection", "category": "marketing", "can_be_overruled": false},
		{"address": "ben@example.test", "reason_code": "no_compatible_evidence", "category": "requested_followup", "requested_category": "active_deal_followup", "can_be_overruled": true},
		{"address": "cy@example.test", "reason_code": "recipient_resolves_to_no_single_subject", "can_be_overruled": false},
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
		decision("unproven@example.test", commsauthz.ReasonNoEvidence, commsauthz.VerdictDeny, commsauthz.ModeEnforce),
		decision("allowed@example.test", commsauthz.ReasonAllowed, commsauthz.VerdictAllow, commsauthz.ModeEnforce),
	}}

	var got []string
	for _, r := range BlockingRefusals(set) {
		got = append(got, fmt.Sprintf("%s overrulable=%t", r.Address, r.CanBeOverruled))
	}
	// Overrulability is the preview's own answer, so both surfaces pick alike.
	// The subject's own objection ranks first.
	want := []string{"objected@example.test overrulable=true", "enforced@example.test overrulable=true", "unproven@example.test overrulable=true"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blocking refusals = %v, want %v; a deny under observe did not stop the send", got, want)
	}
}

// A refused claim keeps both readings on the way to the sender: what the
// engine resolved and what the sender picked. The composer names the deal a
// follow-up needs from the second, because an unproven claim never resolves.
func TestABlockingRefusalCarriesTheSendersOwnClaim(t *testing.T) {
	set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{{
		Recipient: connector.Recipient{Email: "ben@example.test"}, Verdict: commsauthz.VerdictReview,
		ReasonCode: commsauthz.ReasonNoEvidence, Mode: commsauthz.ModeEnforce,
		Requested: commsauthz.CategoryActiveDealFollowup, Resolved: commsauthz.CategoryRequestedFollowup,
	}}}
	got := BlockingRefusals(set)
	if len(got) != 1 {
		t.Fatalf("blocking refusals = %v, want one", got)
	}
	if got[0].RequestedCategory != "active_deal_followup" || got[0].Category != "requested_followup" {
		t.Errorf("refusal = %+v, want requested active_deal_followup beside resolved requested_followup", got[0])
	}
}

// The flat refusal list of a 409 names the law cited for each refusal that has
// one, and leaves the key out for one that has none.
func TestARefusalNamesTheLawItCites(t *testing.T) {
	law := commsauthz.Provision{Instrument: "UWG", Provision: "§ 7(2)", SourceURL: "https://example.org/uwg"}
	refusal := &SendRefusedError{ReviewID: ids.NewV7(), Refusals: []RefusedRecipient{
		{Address: "ana@example.test", ReasonCode: commsauthz.ReasonObjection, Provision: law},
		{Address: "ben@example.test", ReasonCode: commsauthz.ReasonObjection},
	}}
	got, ok := refusal.FaultReference()["refusals"].([]map[string]any)
	if !ok || len(got) != 2 {
		t.Fatalf("refusals on the wire = %#v", refusal.FaultReference()["refusals"])
	}
	cited := fmt.Sprintf("%+v", got[0]["provision"])
	if !strings.Contains(cited, law.Provision) || !strings.Contains(cited, law.SourceURL) {
		t.Errorf("ana's refusal cites %s, want %+v", cited, law)
	}
	if _, present := got[1]["provision"]; present {
		t.Errorf("ben's refusal carries a provision %v, and none was cited", got[1]["provision"])
	}
}
