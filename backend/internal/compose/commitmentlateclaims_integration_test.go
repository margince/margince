// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A late hand-logged meeting files its promise on the customer it was made to
// while the proposal waits on the promiser. Until somebody accepts it, the
// promise stays out of what the customer's record says we owe and stays in
// its history. Accepting makes it current; turning it down withdraws it from
// both (#8718).
func TestALateProposalsCustomerClaimIsHistoryUntilAccepted(t *testing.T) {
	for _, accept := range []bool{true, false} {
		name := map[bool]string{true: "accepted", false: "refused"}[accept]
		t.Run(name, func(t *testing.T) {
			e := setupTranscript(t)
			customer := ids.From[ids.ContactKind](linkInes(t, e))
			// The customer is the promiser's, so accepting may file the promise on them.
			e.WsExec(t, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.Rep1, customer)
			e.WsExec(t, `UPDATE activity SET occurred_at = now() - interval '35 days' WHERE id = $1`, e.activity)

			read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
			if len(read.ProposalIDs) != 1 {
				t.Fatalf("a 35-day-old hand-logged meeting staged %d proposals, want 1 (%v)", len(read.ProposalIDs), read.StatusDetail)
			}
			history, current := customerClaims(t, e, customer)
			if history != 1 || current != 0 {
				t.Fatalf("before an answer the customer holds %d claims and owes %d, want 1 in history and none owed", history, current)
			}

			if accept {
				if _, err := e.svc.Decide(e.ctx, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), true, nil); err != nil {
					t.Fatalf("accepting: %v", err)
				}
				if history, current = customerClaims(t, e, customer); history != 1 || current != 1 {
					t.Errorf("after accepting the customer holds %d claims and owes %d, want the one promise owed", history, current)
				}
				return
			}
			e.rejectFirst(t, read.ProposalIDs)
			if _, current = customerClaims(t, e, customer); current != 0 {
				t.Errorf("after refusing the customer owes %d promises, want none", current)
			}
			if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE contact_id = $1 AND status = 'open' AND archived_at IS NULL`, customer); n != 0 {
				t.Errorf("after refusing %d claims on the customer still read open", n)
			}
		})
	}
}

// customerClaims counts the claims the customer's record lists in its history,
// and those it says we owe.
func customerClaims(t *testing.T, e *transcriptEnv, customer ids.ContactID) (history, current int) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		all, err := e.Contacts.ClaimsForContact(e.Admin(), tx, customer, nil, 20)
		if err != nil {
			return err
		}
		owed, err := e.Contacts.OpenCommitmentsForContact(e.Admin(), tx, customer, nil, 20)
		history, current = len(all), len(owed)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return history, current
}

// A recent promise the reading was unsure of is owed to the customer while it
// waits on its promiser. Turning the proposal down withdraws it there too.
func TestARefusedProposalWithdrawsTheCustomersClaim(t *testing.T) {
	e := setupTranscript(t)
	// The customer is not the promiser's to change, which must not stop them
	// turning their own promise down.
	customer := ids.From[ids.ContactKind](linkInes(t, e))
	read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.75, e.Rep1)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("an unsure promise staged %d proposals, want 1 (%v)", len(read.ProposalIDs), read.StatusDetail)
	}
	if _, current := customerClaims(t, e, customer); current != 1 {
		t.Fatalf("while the proposal waits the customer is owed %d promises, want 1", current)
	}
	e.rejectFirst(t, read.ProposalIDs)
	history, current := customerClaims(t, e, customer)
	if current != 0 || history != 1 {
		t.Errorf("after refusing the customer holds %d claims and is owed %d, want 1 in history and none owed", history, current)
	}
	if got := e.wsString(t, `SELECT status FROM conversation_claim WHERE contact_id = $1`, customer); got != "dismissed" {
		t.Errorf("the refused promise's claim reads %q, want dismissed", got)
	}
	// The extractor that filed the claim withdraws it; the refusal stays the human's.
	if got := e.wsString(t, `SELECT actor_type || ' ' || actor_id FROM audit_log
		WHERE entity_type = 'contact' AND entity_id = $1 AND after->>'claim_status' = 'dismissed'`, customer); got != "system "+signalScanActor {
		t.Errorf("the withdrawal is audited as %q, want the extractor %q", got, signalScanActor)
	}
	if got := e.wsString(t, `SELECT status || ' ' || coalesce(decided_by::text, '') FROM approval WHERE id = $1`, read.ProposalIDs[0]); got != "rejected "+e.Rep1.String() {
		t.Errorf("the proposal reads %q after the refusal, want rejected by the promiser", got)
	}
}
