// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The other reasons a preview offers when the claimed one fails, through the
// real engine.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// theyWroteAMonthAgo plants an authenticated inbound mail from the contact,
// the recorded reason a requested follow-up rests on.
func (e *resolveEnv) theyWroteAMonthAgo(t *testing.T) {
	t.Helper()
	mail := e.inboundFrom(t, "their-question", e.address, clocktest.Now(t).AddDate(0, 0, -30))
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE activity SET captured_by = 'connector:gmail', inbound_sender_authenticated = true
		 WHERE id = $1`, mail); err != nil {
		t.Fatalf("marking the mail captured: %v", err)
	}
}

func (e *resolveEnv) previewOne(t *testing.T, req commsauthz.Request) commsauthz.Decision {
	t.Helper()
	req.Recipients = []connector.Recipient{{Email: e.address}}
	set, err := e.gate.Preview(e.ctx, req)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(set.Decisions) != 1 {
		t.Fatalf("decisions = %d, want one", len(set.Decisions))
	}
	return set.Decisions[0]
}

// A failed Active deal claim from a contact who wrote to us a month ago names
// the follow-up reason their mail carries. The claim itself is untouched: the
// decision still refuses it, and nothing is switched for the sender.
//
// Mutation: return nil from supportedAlternatives, and the first case fails.
func TestAFailedClaimNamesTheReasonTheRecordCarries(t *testing.T) {
	e := setupResolve(t)
	e.theyWroteAMonthAgo(t)
	deal := e.openDeal(t, "open", false)

	d := e.previewOne(t, commsauthz.Request{
		Context: commsauthz.CategoryActiveDealFollowup, Links: []ids.UUID{deal},
	})
	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatal("a deal the recipient is no stakeholder on allowed the claim")
	}
	if !slices.Contains(d.SupportedAlternatives, commsauthz.CategoryRequestedFollowup) {
		t.Fatalf("alternatives = %v, want requested_followup", d.SupportedAlternatives)
	}
	if slices.Contains(d.SupportedAlternatives, commsauthz.CategoryActiveDealFollowup) {
		t.Error("the failed claim is offered as its own alternative")
	}
	if d.Requested != commsauthz.CategoryActiveDealFollowup || d.Resolved == commsauthz.CategoryRequestedFollowup {
		t.Errorf("requested=%q resolved=%q: the preview switched the reason itself", d.Requested, d.Resolved)
	}

	allowed := e.previewOne(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})
	if allowed.Verdict != commsauthz.VerdictAllow || len(allowed.SupportedAlternatives) != 0 {
		t.Fatalf("the offered reason: verdict=%q alternatives=%v, want an allow with none", allowed.Verdict, allowed.SupportedAlternatives)
	}
}

// No reason is offered where none would carry the send: with nothing on
// record, after a stop, and for a marketing claim.
//
// Mutation: drop the Verdict/Resolved test in supportedAlternatives, and the
// empty-record case offers every reason; drop the marketing guard, and the
// marketing case offers the follow-up.
func TestNoAlternativeIsOfferedThatWouldNotCarryTheSend(t *testing.T) {
	t.Run("nothing on record", func(t *testing.T) {
		e := setupResolve(t)
		d := e.previewOne(t, commsauthz.Request{Context: commsauthz.CategoryActiveDealFollowup})
		if len(d.SupportedAlternatives) != 0 {
			t.Fatalf("alternatives = %v, want none", d.SupportedAlternatives)
		}
	})
	t.Run("the address hard-bounced", func(t *testing.T) {
		e := setupResolve(t)
		e.theyWroteAMonthAgo(t)
		e.suppress(t, "hard_bounce")
		d := e.previewOne(t, commsauthz.Request{Context: commsauthz.CategoryActiveDealFollowup})
		if len(d.SupportedAlternatives) != 0 {
			t.Fatalf("alternatives = %v, want none past a stop", d.SupportedAlternatives)
		}
	})
	t.Run("a marketing claim", func(t *testing.T) {
		e := setupResolve(t)
		e.theyWroteAMonthAgo(t)
		d := e.previewOne(t, commsauthz.Request{Context: commsauthz.CategoryMarketing})
		if d.Verdict == commsauthz.VerdictAllow {
			t.Fatal("marketing with no consent was allowed, so this case tests nothing")
		}
		if len(d.SupportedAlternatives) != 0 {
			t.Fatalf("alternatives = %v, want none for marketing", d.SupportedAlternatives)
		}
	})
}
