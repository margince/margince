// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A subject can hold MORE THAN ONE live stop of one kind now that purpose_id
// distinguishes them: a narrow marketing_objection scoped to one newsletter
// and a broad one covering all marketing are different subscriptions, not two
// copies of the same fact. The carry's DISTINCT ON used to key on kind alone,
// which was exactly right when a kind carried no purpose and is wrong once it
// does — collapsing the two to whichever the ORDER BY put first and silently
// dropping the other. A survivor who then loses the narrow stop keeps
// receiving the one list its retiring predecessor asked to leave.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// seedLead inserts a bare lead the withdrawal press can name, the same shape
// TestALeadOnlyRecipientGetsAWorkingOptOut seeds in the sibling file.
func seedLead(t *testing.T, e *channelConsentEnv, name, address string) ids.LeadID {
	t.Helper()
	var leadID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ($1, $2, 'test', 'human:x') RETURNING id`,
		name, address).Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	return ids.From[ids.LeadKind](leadID)
}

// TWO LIVE STOPS OF ONE KIND, DIFFERING ONLY BY PURPOSE, BOTH SURVIVE A CARRY.
//
// The retiring lead pressed two different withdrawal links: one narrowed to
// the newsletter, one an all-marketing sweep. Both write kind=marketing_objection
// — StopForCredentialTx always does, per its own comment — so before purpose_id
// existed these read as the same fact twice. They are not: one subscription is
// stopped, and the other, broader, refusal is a second fact the subject asked
// for separately. A merge that keeps only one of them is exactly the defect
// this package exists to close, just narrower than the kind-only case.
func TestACarryKeepsANarrowAndABroadStopOfOneKindBothOnTheSurvivor(t *testing.T) {
	e := setupChannelConsent(t)
	from := seedLead(t, e, "Two Stops", "two-stops@example.test")
	to := seedLead(t, e, "Two Stops Survivor", "two-stops-survivor@example.test")

	// Seeded through the REAL press, not an INSERT this test invents: the
	// scope a link carries is the scope StopForCredentialTx narrows the row
	// to, and a hand-written row could silently drift from what a press
	// actually produces.
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := e.store.StopForCredentialTx(e.ctx, tx, WithdrawalRef{
			LeadID: from, Address: "two-stops@example.test",
			Scope: WithdrawalScopeNamedPurpose, PurposeID: e.newsletter.UUID,
		}); err != nil {
			return err
		}
		_, err := e.store.StopForCredentialTx(e.ctx, tx, WithdrawalRef{
			LeadID: from, Address: "two-stops@example.test",
			Scope: WithdrawalScopeAllMarketing,
		})
		return err
	}); err != nil {
		t.Fatalf("seeding the two stops: %v", err)
	}

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.CarryStopsTx(e.ctx, tx,
			commsauthz.LeadStopSubject(from), commsauthz.LeadStopSubject(to))
	}); err != nil {
		t.Fatalf("carrying: %v", err)
	}

	rows, err := e.owner.Query(context.Background(), `
		SELECT purpose_id FROM communication_suppression
		 WHERE lead_id = $1 AND kind = $2 AND revoked_at IS NULL`,
		to.UUID, commsauthz.ReasonObjection)
	if err != nil {
		t.Fatalf("reading the survivor's stops: %v", err)
	}
	defer rows.Close()
	var purposes []*ids.UUID
	for rows.Next() {
		var p *ids.UUID
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		purposes = append(purposes, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(purposes) != 2 {
		t.Fatalf("the survivor holds %d live marketing_objection(s), want 2 — a narrow and a "+
			"broad stop differ only by purpose_id and the carry must keep both", len(purposes))
	}
	var sawBroad, sawNarrow bool
	for _, p := range purposes {
		switch {
		case p == nil:
			sawBroad = true
		case *p == e.newsletter.UUID:
			sawNarrow = true
		default:
			t.Errorf("an unexpected purpose_id landed on the survivor: %v", *p)
		}
	}
	if !sawBroad {
		t.Error("the broad (all-marketing) stop did not reach the survivor")
	}
	if !sawNarrow {
		t.Error("the narrow (newsletter-only) stop did not reach the survivor")
	}
}
