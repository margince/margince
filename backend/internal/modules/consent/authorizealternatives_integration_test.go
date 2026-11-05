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

// tableCounts reads the rows a send's decision could write, so a preview can
// be shown to have written none of them.
func (e *resolveEnv) tableCounts(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{
		"communication_basis", "consent_qualifying_event", "audit_log", "event_outbox", "communication_decision",
	} {
		var n int
		if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// Working out an alternative decides the other reason the way a send would,
// and a send under it records a ground. The preview rolls all of it back.
//
// Mutation: commit the preview's transaction instead of rolling it back, and
// the basis and audit counts grow.
func TestWorkingOutAnAlternativeWritesNothing(t *testing.T) {
	e := setupResolve(t)
	e.theyWroteAMonthAgo(t)
	before := e.tableCounts(t)

	d := e.previewOne(t, commsauthz.Request{Context: commsauthz.CategoryActiveDealFollowup})
	if !slices.Contains(d.SupportedAlternatives, commsauthz.CategoryRequestedFollowup) {
		t.Fatalf("alternatives = %v, want requested_followup, or this case tests nothing", d.SupportedAlternatives)
	}
	after := e.tableCounts(t)
	for table, n := range before {
		if after[table] != n {
			t.Errorf("%s: %d rows before the preview, %d after", table, n, after[table])
		}
	}
}

// Each recipient is offered only what its own record carries: one wrote to
// us a month ago, the other's request was recorded and then retracted.
func TestEachRecipientGetsOnlyItsOwnAlternatives(t *testing.T) {
	e := setupResolve(t)
	recorded := e.recordRequest(t, clocktest.Now(t).AddDate(0, 0, -10), false)
	if err := e.store.RetractQualifyingEvent(e.seatCtx(e.user), e.contact, recordedID(t, recorded)); err != nil {
		t.Fatalf("retracting the request: %v", err)
	}
	writer := e.secondContactWhoWroteAMonthAgo(t, "kai@buyer.test")

	set, err := e.gate.Preview(e.ctx, commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}, {Email: writer}},
		Context:    commsauthz.CategoryActiveDealFollowup,
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	offered := map[string][]commsauthz.Category{}
	for _, d := range set.Decisions {
		offered[d.Recipient.Email] = d.SupportedAlternatives
	}
	if len(offered[e.address]) != 0 {
		t.Errorf("the retracted request still offered %v", offered[e.address])
	}
	if !slices.Contains(offered[writer], commsauthz.CategoryRequestedFollowup) {
		t.Errorf("the recipient who wrote was offered %v, want requested_followup", offered[writer])
	}
}

// secondContactWhoWroteAMonthAgo plants another contact with an authenticated
// inbound mail from them, and answers their address.
func (e *resolveEnv) secondContactWhoWroteAMonthAgo(t *testing.T, address string) string {
	t.Helper()
	ctx := context.Background()
	contact, mail := ids.NewV7(), ids.NewV7()
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Kai Buyer', 'manual', 'human:x')`, []any{contact}},
		{`INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by) VALUES ($1, $2, true, 'manual', 'human:x')`, []any{contact, address}},
		{`INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by, inbound_sender_authenticated)
		  VALUES ($1, 'email', 'inbound', 'kai-question', $2, 'gmail', 'connector:gmail', true)`, []any{mail, clocktest.Now(t).AddDate(0, 0, -30)}},
		{`INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`, []any{mail, contact, address}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{mail, contact}},
	} {
		if _, err := e.owner.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatalf("planting the second contact: %v", err)
		}
	}
	return address
}
