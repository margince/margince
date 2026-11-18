// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type keptFixture struct {
	e       *dedupeEnv
	system  context.Context
	human   context.Context
	claim   ids.UUID
	contact ids.UUID
	meeting ids.UUID
}

// keptPromise seeds a promise to invite "Riley Reed", written on Contact C, and
// a later meeting with one guest at the given answer.
func keptPromise(t *testing.T, guest, answer string) keptFixture {
	t.Helper()
	e := setupDedupe(t)
	human := e.as()
	system := principal.WithActor(human, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	f := keptFixture{e: e, system: system, human: human, claim: ids.NewV7(), meeting: ids.NewV7()}
	holder, source, guestID := ids.NewV7(), ids.NewV7(), ids.NewV7()
	f.contact = holder
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES
		    ($1, 'Casey Customer', 'manual', 'human:test'), ($2, $3, 'manual', 'human:test')`, []any{holder, guestID, guest}},
		{`INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at)
		    VALUES ($1, 'email', 'Next steps', 'manual', 'human:test', now() - interval '1 day')`, []any{source}},
		{`INSERT INTO conversation_claim
		    (id, contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by)
		    VALUES ($1, $2, 'commitment_ours', 'I will invite Riley Reed to the demo', $3,
		            'I will invite Riley Reed', 'fp-kept', 'manual', 'human:test')`, []any{f.claim, holder, source}},
		{`INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at, meeting_status)
		    VALUES ($1, 'meeting', 'Demo', 'manual', 'human:test', now() + interval '2 days', 'booked')`, []any{f.meeting}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role, response)
		    VALUES ($1, $2, 'attendee', $3)`, []any{f.meeting, guestID, answer}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role)
		    VALUES ($1, $2, 'attendee')`, []any{f.meeting, holder}},
	} {
		e.execAs(human, t, stmt.sql, stmt.args...)
	}
	return f
}

func (f keptFixture) status(t *testing.T) string {
	t.Helper()
	var status string
	if err := f.e.store.tx(f.human, func(tx pgx.Tx) error {
		return tx.QueryRow(f.human, `SELECT status FROM conversation_claim WHERE id = $1`, f.claim).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f keptFixture) kept(t *testing.T) []ids.UUID {
	t.Helper()
	found, err := f.e.store.KeptByAcceptedGuest(f.system, f.meeting)
	if err != nil {
		t.Fatalf("KeptByAcceptedGuest: %v", err)
	}
	return found
}

func TestAnAcceptedInviteToTheNamedContactClosesThePromiseAndItCanBeUndone(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	found := f.kept(t)
	if len(found) != 1 || found[0] != f.claim {
		t.Fatalf("kept promises = %v, want the invite promise", found)
	}
	if err := f.e.store.SettleKeptByEvidence(f.system, f.human, f.claim, f.meeting); err != nil {
		t.Fatalf("settling on evidence: %v", err)
	}
	if got := f.status(t); got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
	audited := f.e.countAs(f.human, t, `
		SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1
		   AND after->>'claim_status' = 'done' AND after->>'settled_by' = 'automatic'
		   AND after->>'evidence_activity_id' = $2`, f.claim.String(), f.meeting.String())
	if audited != 1 {
		t.Errorf("automatic settlement audit rows = %d, want 1 naming the meeting", audited)
	}

	if err := f.e.store.ReopenAutoSettledClaim(f.human, f.claim); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := f.status(t); got != "open" {
		t.Errorf("status after undo = %q, want open", got)
	}
	if again := f.kept(t); len(again) != 0 {
		t.Errorf("a promise a human reopened is found kept again: %v", again)
	}
}

func TestAnInviteToSomeoneElseOrNotAcceptedClosesNothing(t *testing.T) {
	for name, tc := range map[string]struct{ guest, answer string }{
		"another contact": {"Jordan Other", "accepted"},
		"first name only": {"Riley", "accepted"},
		"declined":        {"Riley Reed", "declined"},
		"no answer yet":   {"Riley Reed", "needs_action"},
	} {
		t.Run(name, func(t *testing.T) {
			f := keptPromise(t, tc.guest, tc.answer)
			if found := f.kept(t); len(found) != 0 {
				t.Errorf("kept promises = %v, want none", found)
			}
		})
	}
}

func TestOnlyTheSystemSettlesOnEvidenceAndOnlyAutomaticSettlementsUndo(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	if err := f.e.store.SettleKeptByEvidence(f.human, f.human, f.claim, f.meeting); err == nil {
		t.Error("a human settled through the evidence path")
	}
	if err := f.e.store.SettleConversationClaim(f.human, f.claim, "done"); err != nil {
		t.Fatalf("a human's settle: %v", err)
	}
	if err := f.e.store.ReopenAutoSettledClaim(f.human, f.claim); err == nil {
		t.Error("the undo opened a promise a human settled")
	}
}

func TestAMeetingTheClaimsContactIsNotOnSettlesNothing(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	f.e.execAs(f.human, t, `DELETE FROM activity_participant WHERE activity_id = $1 AND response IS NULL`, f.meeting)
	if found := f.kept(t); len(found) != 0 {
		t.Errorf("an unrelated meeting with a matching name kept %v", found)
	}
}

func TestOnlyAnInternalSeatsMeetingCanSettle(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	if _, internal, err := f.e.store.MeetingSeat(f.system, f.meeting); err != nil || internal {
		t.Fatalf("an invitation from outside has a seat: %v / %v", internal, err)
	}
	f.e.execAs(f.human, t, `INSERT INTO activity_participant (activity_id, user_id, role)
		VALUES ($1, $2, 'organizer')`, f.meeting, f.e.rep)
	seat, internal, err := f.e.store.MeetingSeat(f.system, f.meeting)
	if err != nil || !internal || seat != f.e.rep {
		t.Fatalf("seat = %v %v %v, want the organizing colleague", seat, internal, err)
	}
}

func TestASeatWithoutWriteAccessToTheClaimsContactSettlesNothing(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	readOnly := principal.WithActor(f.human, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + f.e.rep.String(), UserID: f.e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"viewer"},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}},
		},
	})
	if err := f.e.store.SettleKeptByEvidence(f.system, readOnly, f.claim, f.meeting); err == nil {
		t.Fatal("a seat that may not update contacts settled the promise")
	}
	if got := f.status(t); got != "open" {
		t.Errorf("status = %q, want open", got)
	}
}

func TestACalledOffMeetingKeepsNoPromiseAndTakesBackTheOneItKept(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	if err := f.e.store.SettleKeptByEvidence(f.system, f.human, f.claim, f.meeting); err != nil {
		t.Fatalf("settling: %v", err)
	}
	f.e.execAs(f.human, t, `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, f.meeting)
	if err := f.e.store.ReopenKeptByCalledOffMeeting(f.system, f.meeting); err != nil {
		t.Fatalf("taking back: %v", err)
	}
	if got := f.status(t); got != "open" {
		t.Errorf("status after the meeting was called off = %q, want open", got)
	}
	if found := f.kept(t); len(found) != 0 {
		t.Errorf("a called-off meeting kept %v", found)
	}
}

func TestAHumanSettledPromiseSurvivesACalledOffMeeting(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	if err := f.e.store.SettleConversationClaim(f.human, f.claim, "done"); err != nil {
		t.Fatal(err)
	}
	f.e.execAs(f.human, t, `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, f.meeting)
	if err := f.e.store.ReopenKeptByCalledOffMeeting(f.system, f.meeting); err != nil {
		t.Fatal(err)
	}
	if got := f.status(t); got != "done" {
		t.Errorf("status = %q, want done: a human settled it", got)
	}
}

func TestTheClaimSaysItWasSettledAutomatically(t *testing.T) {
	f := keptPromise(t, "Riley Reed", "accepted")
	if err := f.e.store.SettleKeptByEvidence(f.system, f.human, f.claim, f.meeting); err != nil {
		t.Fatal(err)
	}
	var claims []crmcontracts.ConversationClaim
	if err := f.e.store.tx(f.human, func(tx pgx.Tx) error {
		var err error
		claims, err = f.e.store.ClaimsForContact(f.human, tx, ids.From[ids.ContactKind](f.contact), nil, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].SettledAutomatically == nil || !*claims[0].SettledAutomatically {
		t.Fatalf("claims = %+v, want one marked settled automatically", claims)
	}
	if err := f.e.store.ReopenAutoSettledClaim(f.human, f.claim); err != nil {
		t.Fatal(err)
	}
}
