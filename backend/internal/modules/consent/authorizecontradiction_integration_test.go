// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A caller who says what a message IS and names a purpose meaning something
// else is refused, rather than quietly sent under the purpose's reading.
//
// The reading was the escape path. A send claiming `marketing` under the
// `business_correspondence` key resolved to reply_to_inbound, which the
// correspondence arm authorizes on any recent exchange with that contact — not
// evidence that THIS message is a reply to anything. So promotional content
// went out as correspondence.
//
// And it went out past the subject. An Art. 21 objection binds
// CategoryMarketing and is tested against the RESOLVED category, so a message
// remapped to reply_to_inbound was never put to it: the stop was recorded,
// appeared in the subject's own export, and did not stop the mail.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// claimingMarketingUnder is the request at the heart of this: the caller names
// the category and a purpose key from another class.
func claimingMarketingUnder(purposeKey string) commsauthz.Request {
	return commsauthz.Request{
		Context:          commsauthz.CategoryMarketing,
		LegacyPurposeKey: purposeKey,
	}
}

// The escape path itself, refused.
func TestAClaimContradictedByItsPurposeIsRefused(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "business_correspondence", "business_correspondence")

	d := e.decide(t, claimingMarketingUnder("business_correspondence"))

	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny — a request that claims one category and names a "+
			"purpose meaning another asks two things at once", d.Verdict)
	}
	if d.ReasonCode != commsauthz.ReasonPurposeContradictsClaim {
		t.Errorf("reason = %q, want %q", d.ReasonCode, commsauthz.ReasonPurposeContradictsClaim)
	}
	// The row says BOTH things. Requested is what the caller asked for, so an
	// auditor can see that promotional content was claimed; Resolved stays the
	// engine's own reading, because an unproven claim must never reach the
	// column that selects the rollout mode and counts the advertising ceiling.
	if d.Requested != commsauthz.CategoryMarketing {
		t.Errorf("requested = %q, want %q — the claim has to survive into the row",
			d.Requested, commsauthz.CategoryMarketing)
	}
	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved = %q, want %q — the engine's own reading, never the claim",
			d.Resolved, commsauthz.CategoryReplyToInbound)
	}
}

// The half that makes it a privacy defect rather than a tidiness one: with the
// remap in place the objection was never reached.
func TestAMarketingObjectionIsNotWalkedPastByACorrespondencePurpose(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "business_correspondence", "business_correspondence")
	e.suppress(t, "marketing_objection")
	// A qualifying event on ANOTHER conversation, which is exactly what the
	// correspondence arm authorizes on: any recent inbound from this contact,
	// not evidence that the message being sent is a reply to anything. On a
	// thread of its own so the request's own resolution cannot find it — this
	// send has no anchor, so nothing resolves it before the purpose does.
	e.inboundFrom(t, "an-unrelated-thread", e.address, time.Now().Add(-time.Hour))

	d := e.decide(t, claimingMarketingUnder("business_correspondence"))

	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny — this contact objected to direct marketing and the "+
			"caller said this message is direct marketing", d.Verdict)
	}
	// Refused before the suppression is even reached, which is the stronger
	// answer: the request never becomes a message to test an objection
	// against. What matters is that it does not SEND.
	if d.ReasonCode == commsauthz.ReasonAllowed {
		t.Errorf("reason = %q, and this message went out past an Art. 21 objection", d.ReasonCode)
	}
}

// A claim the purpose AGREES with still goes through the legacy lane it always
// did. The refusal is about disagreement, not about naming both.
func TestAClaimItsPurposeAgreesWithIsStillDecidedByTheLegacyLane(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")

	d := e.decide(t, commsauthz.Request{
		Context:          commsauthz.CategoryMarketing,
		LegacyPurposeKey: "newsletter",
	})

	if d.ReasonCode == commsauthz.ReasonPurposeContradictsClaim {
		t.Fatalf("a marketing claim under a marketing purpose was called a contradiction")
	}
	if d.Resolved != commsauthz.CategoryMarketing {
		t.Errorf("resolved = %q, want %q", d.Resolved, commsauthz.CategoryMarketing)
	}
}

// A caller who claims NOTHING is the legacy path, and it is untouched: the
// purpose alone says what the message is, and there is nothing to contradict.
func TestAPurposeWithNoClaimStillSpeaksForTheMessage(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "business_correspondence", "business_correspondence")

	d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "business_correspondence"})

	if d.ReasonCode == commsauthz.ReasonPurposeContradictsClaim {
		t.Fatalf("a request that claimed nothing was called a contradiction — the legacy path " +
			"names only a purpose, and refusing it would refuse every message that has not " +
			"moved to the category vocabulary")
	}
	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved = %q, want %q — the purpose's own class speaks when nobody else does",
			d.Resolved, commsauthz.CategoryReplyToInbound)
	}
}
