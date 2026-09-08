// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// verdict.go's own answer must not disagree with the transmit gate's: a person
// who asked to stop must not read as VerdictAllowed a moment after
// authorizetransmit.go's liveSuppression would refuse the same send.

import (
	"testing"
	"time"
)

// suppress records a communication_suppression row the way suppress.go's
// writer does, minus the write's own auth/audit machinery — this test is about
// what VerdictForPerson reads, not about who may write the row.
func (e *qualifyingEnv) suppress(t *testing.T, kind string) {
	t.Helper()
	seedLiveSuppression(e.ctx, t, e.owner, e.person, kind, "phone call")
}

func TestASubjectRequestBlocksCorrespondenceTheGuardWouldOtherwiseAllow(t *testing.T) {
	e := setupQualifying(t)
	e.inbound(t, time.Now().Add(-24*time.Hour))

	// Before the suppression: the inbound message is a qualifying event, so
	// this reads allowed.
	if got := e.verdict(t); got.State != VerdictAllowed {
		t.Fatalf("before the suppression: state = %q, want %q", got.State, VerdictAllowed)
	}

	e.suppress(t, "subject_request")

	got := e.verdict(t)
	if got.State != VerdictBlocked {
		t.Fatalf("after a subject_request suppression: state = %q, want %q (verdict.go must agree with authorizetransmit.go's liveSuppression)", got.State, VerdictBlocked)
	}
	if got.Code != BlockSuppressed {
		t.Errorf("code = %q, want %q", got.Code, BlockSuppressed)
	}
	if got.Reason == "" {
		t.Error("a blocked verdict must say why, in words a rep can repeat")
	}
}

// A marketing objection binds ONLY marketing (suppressionBinds's own rule) —
// business correspondence must stay open, or this fix would over-block the
// overwhelming majority of ordinary replies the moment anybody unsubscribed
// from a newsletter.
func TestAMarketingObjectionDoesNotBlockBusinessCorrespondence(t *testing.T) {
	e := setupQualifying(t)
	e.inbound(t, time.Now().Add(-24*time.Hour))
	e.suppress(t, "marketing_objection")

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("state = %q, want %q — a marketing objection must not reach business correspondence", got.State, VerdictAllowed)
	}
}

// A statutory restriction (Art. 18) binds business correspondence too — it is
// not one of the three subject-serving categories Art. 18(2) exempts.
func TestAProcessingRestrictionBlocksCorrespondence(t *testing.T) {
	e := setupQualifying(t)
	e.inbound(t, time.Now().Add(-24*time.Hour))
	e.suppress(t, "processing_restriction")

	got := e.verdict(t)
	if got.State != VerdictBlocked {
		t.Fatalf("state = %q, want %q — a processing restriction binds business correspondence", got.State, VerdictBlocked)
	}
	if got.Reason == "" {
		t.Error("a blocked verdict must say why")
	}
}

// A hard bounce is a fact about a MAILBOX (liveSuppression's own doc comment)
// and is recorded against the address alone, never a person — this guard
// answers about the person in general, with no address of its own to check,
// so a bounce on one of their addresses must not read as "this person is
// blocked" when another channel might still reach them.
func TestAnAddressPinnedHardBounceDoesNotBlockThePersonLevelVerdict(t *testing.T) {
	e := setupQualifying(t)
	e.inbound(t, time.Now().Add(-24*time.Hour))
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level)
		VALUES ('dead-mailbox@example.test', 'hard_bounce', 'provider', 'human:x', 'machine')`,
	); err != nil {
		t.Fatalf("seeding the address-pinned bounce: %v", err)
	}

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("state = %q, want %q — an address-pinned bounce is not a fact about this person", got.State, VerdictAllowed)
	}
}

// A revoked suppression is not a live one — liveSuppression's own WHERE
// clause already excludes it, and this pins that VerdictForPerson inherits
// that read rather than a second, looser one.
func TestARevokedSuppressionDoesNotBlock(t *testing.T) {
	e := setupQualifying(t)
	e.inbound(t, time.Now().Add(-24*time.Hour))
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (person_id, kind, source, captured_by, decided_by_level, revoked_at)
		VALUES ($1, 'subject_request', 'phone call', 'human:x', 'subject', now())`,
		e.person); err != nil {
		t.Fatalf("seeding the revoked suppression: %v", err)
	}

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("state = %q, want %q — a revoked suppression must not block", got.State, VerdictAllowed)
	}
}
