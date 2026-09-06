// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What each kind of suppression binds, measured against a real resolved
// category. Three kinds with three different reaches: applying the strongest to
// every message refuses mail nobody objected to.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// suppress records one live suppression against the env's person.
func (e *resolveEnv) suppress(t *testing.T, kind string) {
	t.Helper()
	// decided_by_level as the machinery writes it: an objection and a
	// restriction are legal facts and a bounce is the provider's answer, so
	// none of the three is a user-level decision.
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression
		    (person_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, $2, 'test', 'human:x', 'subject')`, e.person, kind); err != nil {
		t.Fatalf("recording the %s: %v", kind, err)
	}
}

// AN ART. 21 OBJECTION IS ABOUT MARKETING, AND BINDS MARKETING.
//
// It beats consent, the German existing-customer exception and every other
// basis for that category, and no mode softens it.
func TestAMarketingObjectionStopsMarketing(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.suppress(t, "marketing_objection")

	d := e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})
	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: they objected to exactly this", d.Verdict)
	}
	if d.ReasonCode != commsauthz.ReasonObjection {
		t.Errorf("reason = %q, want %q", d.ReasonCode, commsauthz.ReasonObjection)
	}
}

// AND IT DOES NOT STOP THEIR INVOICE.
//
// This is the defect the split exists for: the objection was applied before the
// category was known, so an Art. 21 objection to marketing refused the invoice
// the same person is owed. An objection to direct marketing says nothing about
// a message the law requires us to send.
func TestAMarketingObjectionDoesNotStopAReply(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, "marketing_objection")

	// A thread the subject wrote into: the reply arm, on their own evidence.
	anchor := e.inboundFrom(t, "thread-invoice", e.address, time.Now().Add(-24*time.Hour))

	d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor})
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: they objected to MARKETING, and this is a reply "+
			"to their own message", d.Verdict, d.ReasonCode)
	}
	// The objection is still on the record, which is what a later reader needs.
	if d.Suppression != "marketing_objection" {
		t.Errorf("suppression = %q, want the objection recorded on the row even though it did not bind",
			d.Suppression)
	}
}

// A PROCESSING RESTRICTION BINDS EVERYTHING THE SUBJECT IS NOT OWED.
func TestAProcessingRestrictionStopsOrdinaryCorrespondence(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, "processing_restriction")
	anchor := e.inboundFrom(t, "thread-restricted", e.address, time.Now().Add(-24*time.Hour))

	d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor})
	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: a restriction stops correspondence", d.Verdict)
	}
	if d.ReasonCode != commsauthz.ReasonRestricted {
		t.Errorf("reason = %q, want %q", d.ReasonCode, commsauthz.ReasonRestricted)
	}
}

// A HARD BOUNCE STOPS EVEN WHAT THE SUBJECT IS OWED.
//
// No template makes a dead address accept mail, so the five subject-serving
// categories are not an exception here — a message nobody receives serves them
// no better than one nobody sent.
func TestAHardBounceStopsEvenASubjectServingMessage(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, "hard_bounce")

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategorySecurityNotice})
	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny: the address does not accept mail", d.Verdict)
	}
	if d.ReasonCode != commsauthz.ReasonHardBounce {
		t.Errorf("reason = %q, want %q", d.ReasonCode, commsauthz.ReasonHardBounce)
	}
}

// SOMEBODY WHO PHONED AND SAID STOP IS NOT SENT A RECORD CONFIRMATION.
//
// subject_request is the ONLY kind a seat may write, so it is the kind a real
// installation actually holds. An earlier version of the rules folded it into
// the statutory restriction and let the five subject-serving categories through
// — and POST /people/{id}/consent/confirm-request mails record_confirmation
// with no further consent question, so the person who asked us to stop received
// an invitation to review their record.
func TestSomebodyWhoAskedUsToStopIsNotSentARecordConfirmation(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, "subject_request")

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRecordConfirmation})
	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s): they asked us to stop writing, and this is mail they "+
			"did not ask for", d.ReasonCode)
	}
	if d.ReasonCode != commsauthz.ReasonSubjectRequest {
		t.Errorf("reason = %q, want %q: calling their request a statutory restriction misstates "+
			"a legal fact on a row they can obtain", d.ReasonCode, commsauthz.ReasonSubjectRequest)
	}
}

// A RESTRICTED SUBJECT GETS NO NEW BASIS ROW WRITTEN ABOUT THEM.
//
// Resolution now runs before the suppression is applied, so the category is
// known — but a communication_basis row asserts we hold a lawful ground to
// write to this person, and writing one about a restricted subject is itself
// processing, disclosed back to them under Art. 15.
func TestARestrictedSubjectGetsNoNewBasisRow(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, "processing_restriction")
	anchor := e.inboundFrom(t, "thread-nobasis", e.address, time.Now().Add(-24*time.Hour))

	if d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor}); d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny", d.Verdict)
	}
	var rows int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("communication_basis rows = %d, want 0: a ground was recorded for a subject "+
			"whose processing is restricted", rows)
	}
}

// A WEAKER SUPPRESSION DOES NOT MASK A STRONGER ONE.
//
// The reader used to take a single row ordered by a fixed strength, which was
// sound while every kind refused everything. It stopped being sound when reach
// became category-dependent: a marketing objection sorts first and binds the
// LEAST, so a person carrying both an objection and a hard bounce had the
// bounce masked and their invoice sent to a dead mailbox.
func TestAnObjectionDoesNotMaskAHardBounce(t *testing.T) {
	e := setupResolve(t)
	// Recorded objection-first, which is the order the old strength sort put
	// first and the order that hid the bounce.
	e.suppress(t, "marketing_objection")
	e.suppress(t, "hard_bounce")
	anchor := e.inboundFrom(t, "thread-masked", e.address, time.Now().Add(-24*time.Hour))

	d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor})
	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q (%s), want deny: the objection does not bind a reply, but the "+
			"hard bounce binds everything and was read past", d.Verdict, d.ReasonCode)
	}
	if d.ReasonCode != commsauthz.ReasonHardBounce {
		t.Errorf("reason = %q, want %q: the binding kind is the one that answers",
			d.ReasonCode, commsauthz.ReasonHardBounce)
	}
}

// A WITHDRAWAL STOPS A THREAD-EVIDENCED SEND.
//
// The evidence arms allow on the record's own ground and never read
// person_consent — that is what makes a reply to a thread the subject started
// work without a consent row. But a subject who presses one-click unsubscribe
// writes a WITHDRAWAL, not a suppression row, so liveSuppression is blind to it
// and the thread arm would allow the very message they just stopped.
func TestAWithdrawalStopsAThreadEvidencedSend(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "business_correspondence", "business_correspondence")
	anchor := e.inboundFrom(t, "thread-withdrawn", e.address, time.Now().Add(-24*time.Hour))

	// They wrote to us, then took their permission back.
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO person_consent (person_id, purpose_id, state, lawful_basis, captured_at, source)
		SELECT $1, id, 'withdrawn', 'consent', now(), 'preference_centre'
		  FROM consent_purpose WHERE key = 'business_correspondence'`, e.person); err != nil {
		t.Fatalf("recording the withdrawal: %v", err)
	}

	d := e.decide(t, commsauthz.Request{
		AnchorActivityID: anchor, LegacyPurposeKey: "business_correspondence",
	})
	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s, resolved %s): they took their permission back and the "+
			"thread arm sent anyway — the evidence arms never read person_consent",
			d.ReasonCode, d.Resolved)
	}
}

// ARCHIVING A PURPOSE DOES NOT REACTIVATE THE PEOPLE WHO STOPPED IT.
//
// A withdrawal is a thing the subject did; archiving the purpose is a thing the
// installation did. If the withdrawal read skipped archived purposes, retiring
// one would silently make everybody who had unsubscribed from it contactable
// again — on the evidence arms, which never read person_consent otherwise.
func TestArchivingAPurposeKeepsItsWithdrawals(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "business_correspondence", "business_correspondence")
	anchor := e.inboundFrom(t, "thread-archived", e.address, time.Now().Add(-24*time.Hour))

	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO person_consent (person_id, purpose_id, state, lawful_basis, captured_at, source)
		SELECT $1, id, 'withdrawn', 'consent', now(), 'preference_centre'
		  FROM consent_purpose WHERE key = 'business_correspondence'`, e.person); err != nil {
		t.Fatalf("recording the withdrawal: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE consent_purpose SET archived_at = now() WHERE key = 'business_correspondence'`); err != nil {
		t.Fatalf("archiving the purpose: %v", err)
	}

	d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor})
	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s): retiring the purpose reactivated somebody who had "+
			"stopped it", d.ReasonCode)
	}
}
