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
)

type looksKeptFixture struct {
	e       *dedupeEnv
	ctx     context.Context
	holder  ids.UUID
	guest   ids.UUID
	claim   ids.UUID
	source  ids.UUID
	meeting ids.UUID
}

// looksKeptPromise seeds a promise to invite "Riley Reed" on Contact C, both at
// one company. It also seeds a later meeting that Riley accepted, with C on it.
func looksKeptPromise(t *testing.T) looksKeptFixture {
	t.Helper()
	e := setupDedupe(t)
	f := looksKeptFixture{
		e: e, ctx: e.as(), holder: ids.NewV7(), guest: ids.NewV7(),
		claim: ids.NewV7(), source: ids.NewV7(), meeting: ids.NewV7(),
	}
	company := ids.NewV7()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Acme', 'manual', 'human:test')`, []any{company}},
		{`INSERT INTO contact (id, full_name, source, captured_by) VALUES
		    ($1, 'Casey Customer', 'manual', 'human:test'), ($2, 'Riley Reed', 'manual', 'human:test')`, []any{f.holder, f.guest}},
		{`INSERT INTO relationship (contact_id, company_id, kind, is_current_primary, source, captured_by)
		    VALUES ($1, $3, 'employment', true, 'manual', 'human:test'), ($2, $3, 'employment', true, 'manual', 'human:test')`, []any{f.holder, f.guest, company}},
		{`INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at)
		    VALUES ($1, 'email', 'Next steps', 'manual', 'human:test', now() - interval '3 days')`, []any{f.source}},
		{`INSERT INTO conversation_claim
		    (id, contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by)
		    VALUES ($1, $2, 'commitment_ours', 'I will invite Riley Reed to the demo', $3,
		            'I will invite Riley Reed', 'fp-looks-kept', 'manual', 'human:test')`, []any{f.claim, f.holder, f.source}},
		{`INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at, meeting_status)
		    VALUES ($1, 'meeting', 'Demo', 'manual', 'human:test', now() + interval '2 days', 'booked')`, []any{f.meeting}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role, response)
		    VALUES ($1, $2, 'attendee', 'accepted')`, []any{f.meeting, f.guest}},
		{`INSERT INTO activity_participant (activity_id, contact_id, role)
		    VALUES ($1, $2, 'attendee')`, []any{f.meeting, f.holder}},
	} {
		e.execAs(f.ctx, t, stmt.sql, stmt.args...)
	}
	return f
}

func (f looksKeptFixture) hint(t *testing.T) *string {
	t.Helper()
	var claims []crmcontracts.ConversationClaim
	if err := f.e.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		claims, err = f.e.store.ClaimsForContact(f.ctx, tx, ids.From[ids.ContactKind](f.holder), nil, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, claim := range claims {
		if claim.LooksKept != nil {
			return &claim.LooksKept.GuestName
		}
	}
	return nil
}

func TestAnAcceptedLaterMeetingMakesThePromiseLookKeptAndSettlesNothing(t *testing.T) {
	f := looksKeptPromise(t)
	guest := f.hint(t)
	if guest == nil || *guest != "Riley Reed" {
		t.Fatalf("hint = %v, want Riley Reed", guest)
	}
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM conversation_claim WHERE id = $1 AND status = 'open'`, f.claim); got != 1 {
		t.Errorf("the claim is not open any more: %d", got)
	}
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1`, f.claim.String()); got != 0 {
		t.Errorf("reading the hint wrote %d audit rows", got)
	}
	if err := f.e.store.SettleConversationClaim(f.ctx, f.claim, "done"); err != nil {
		t.Fatalf("Mark kept goes through the human settle: %v", err)
	}
	if got := f.e.countAs(f.ctx, t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1 AND after->>'claim_status' = 'done'`, f.claim.String()); got != 1 {
		t.Errorf("settle audit rows = %d, want 1", got)
	}
}

func TestADifferentGuestWithTheSameNameAtAnotherCompanyIsNotTheOneInvited(t *testing.T) {
	f := looksKeptPromise(t)
	f.e.execAs(f.ctx, t, `DELETE FROM relationship WHERE contact_id = $1`, f.guest)
	other := ids.NewV7()
	f.e.execAs(f.ctx, t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Globex', 'manual', 'human:test')`, other)
	f.e.execAs(f.ctx, t, `INSERT INTO relationship (contact_id, company_id, kind, is_current_primary, source, captured_by)
		VALUES ($1, $2, 'employment', true, 'manual', 'human:test')`, f.guest, other)
	if got := f.hint(t); got != nil {
		t.Errorf("a namesake at another company suggested %q", *got)
	}
}

func TestOnlyAnAcceptedLiveLaterMeetingCounts(t *testing.T) {
	for name, change := range map[string]string{
		"declined":           `UPDATE activity_participant SET response = 'declined' WHERE activity_id = $1 AND response IS NOT NULL`,
		"guest removed":      `DELETE FROM activity_participant WHERE activity_id = $1 AND response IS NOT NULL`,
		"called off":         `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`,
		"archived":           `UPDATE activity SET archived_at = now() WHERE id = $1`,
		"before the promise": `UPDATE activity SET occurred_at = now() - interval '10 days' WHERE id = $1`,
		"not yet answered":   `UPDATE activity_participant SET response = 'needs_action' WHERE activity_id = $1 AND response IS NOT NULL`,
	} {
		t.Run(name, func(t *testing.T) {
			f := looksKeptPromise(t)
			f.e.execAs(f.ctx, t, change, f.meeting)
			if got := f.hint(t); got != nil {
				t.Errorf("%s still suggested %q", name, *got)
			}
		})
	}
}

func TestAMeetingTheClaimsContactIsNotOnSuggestsNothing(t *testing.T) {
	f := looksKeptPromise(t)
	f.e.execAs(f.ctx, t, `DELETE FROM activity_participant WHERE activity_id = $1 AND contact_id = $2`, f.meeting, f.holder)
	if got := f.hint(t); got != nil {
		t.Errorf("an unrelated meeting suggested %q", *got)
	}
}

func TestAReaderWhoCannotSeeTheMeetingOrTheSourceGetsNoSuggestion(t *testing.T) {
	f := looksKeptPromise(t)
	f.e.execAs(f.ctx, t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, f.meeting)
	if got := f.hint(t); got != nil {
		t.Errorf("a meeting the reader may not open suggested %q", *got)
	}
	g := looksKeptPromise(t)
	g.e.execAs(g.ctx, t, `UPDATE activity SET archived_at = now() WHERE id = $1`, g.source)
	if got := g.hint(t); got != nil {
		t.Errorf("a promise whose source is gone suggested %q", *got)
	}
}
