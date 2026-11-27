// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A promise to invite someone, once an accepted later meeting shows it kept, is
// closed by Margince for the seat that owns it. The close touches only the
// claim, so reopening it restores everything. The audit row of the close carries
// the marker, so the table gets no column.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PromiseKeptActor stamps the audit and event rows of an automatic close, so
// the settle consumer can tell it from a human's settlement.
const PromiseKeptActor = "system:promise-kept"

// autoKeptKey marks the audit image of an automatic close.
const autoKeptKey = "auto_kept"

// recheckWindow is how long after the close it is still compared with the
// meetings. The audit image holds only a marker, so no meeting detail sits in
// the record history for a reader who may not open the meeting.
const recheckWindow = 90 * 24 * time.Hour

// latestClaimImage is the audit image of the claim's newest status change. It
// is found by claim id. A task link written afterwards, or a merge that moved
// the claim to another contact, does not hide who closed it.
const latestClaimImage = `(SELECT a.after FROM audit_log a WHERE a.entity_type = 'contact'
	AND a.after->>'claim_id' = c.id::text AND a.after ? 'claim_status'
	ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1)`

// reopenedByAHuman holds a claim someone took out of done. A human saying a
// promise is not kept outranks what the meeting shows.
var reopenedByAHuman = `EXISTS (SELECT 1 FROM audit_log h WHERE h.entity_type = 'contact'
	AND h.after->>'claim_id' = c.id::text AND h.after ? 'claim_status'
	AND h.before->>'claim_status' = 'done' AND h.after->>'claim_status' = 'open' AND ` + storekit.AuditForAUser("h") + `)`

// KeptSeats answers who is responsible for a promise and what that seat may read.
type KeptSeats interface {
	// Owner is the seat the promise belongs to, or the zero id. named is the
	// seat the claim itself names (its task's assignee or who took it over).
	Owner(ctx context.Context, tx pgx.Tx, source ids.UUID, named *ids.UUID) (ids.UUID, error)
	// Reader is ctx bound to the owner's authority as of this transaction.
	Reader(ctx context.Context, tx pgx.Tx, owner ids.UUID) (context.Context, error)
}

// KeptPromiseCandidatesTx lists the open promises to invite someone and the
// automatic closes still inside their recheck window. Each is decided on its
// own by ReconcileKeptPromiseTx.
func (s *Store) KeptPromiseCandidatesTx(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id FROM conversation_claim c
		 WHERE c.kind = 'commitment_ours' AND NOT c.needs_review
		   AND c.body ~* '\yinvit'
		   AND ((c.status = 'open' AND NOT `+reopenedByAHuman+`)
		     OR (c.status = 'done' AND `+latestClaimImage+` ? '`+autoKeptKey+`' AND c.updated_at > $1))
		 ORDER BY c.id`, s.clock().Add(-recheckWindow))
	if err != nil {
		return nil, fmt.Errorf("read the promises to check against meetings: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// keptPromiseRow is what a decision about one promise reads from it.
type keptPromiseRow struct {
	status          string
	contact, source ids.UUID
	assignee, taker *ids.UUID
	auto, reopened  bool
}

// lockKeptPromise locks the claim and reads it. It answers false for a claim
// that is gone or is not a live promise to invite someone.
func lockKeptPromise(ctx context.Context, tx pgx.Tx, claim ids.UUID) (keptPromiseRow, bool, error) {
	var row keptPromiseRow
	if _, err := storekit.LockRow(ctx, tx, "conversation_claim", claim, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return row, false, nil
		}
		return row, false, err
	}
	err := tx.QueryRow(ctx, `
		SELECT c.status, c.contact_id, c.source_activity_id, t.assignee_id, c.taken_by_user_id,
		       coalesce(`+latestClaimImage+` ? '`+autoKeptKey+`', false), `+reopenedByAHuman+`
		  FROM conversation_claim c
		  LEFT JOIN activity t ON t.id = c.task_activity_id AND t.archived_at IS NULL
		 WHERE c.id = $1 AND c.kind = 'commitment_ours' AND NOT c.needs_review
		   AND c.body ~* '\yinvit'`, claim).Scan(&row.status, &row.contact, &row.source, &row.assignee, &row.taker, &row.auto, &row.reopened)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	if err != nil {
		return row, false, fmt.Errorf("read the promise to check against meetings: %w", err)
	}
	return row, true, nil
}

// ReconcileKeptPromiseTx decides one promise in one transaction. It locks the
// claim, reads the owner, their authority and the evidence under that lock, and
// writes the status. An automatic close whose evidence is gone is reopened. An
// open promise the owner's meetings show kept is closed.
func (s *Store) ReconcileKeptPromiseTx(ctx context.Context, tx pgx.Tx, claim ids.UUID, seats KeptSeats) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	row, found, err := lockKeptPromise(ctx, tx, claim)
	if err != nil || !found {
		return err
	}
	closing := row.status == claimStatusOpen && !row.reopened
	if !closing && (row.status != claimStatusDone || !row.auto) {
		return nil
	}
	named := row.assignee
	if named == nil {
		named = row.taker
	}
	owner, err := seats.Owner(ctx, tx, row.source, named)
	if err != nil {
		return err
	}
	hit, shown, err := s.evidenceForOwner(ctx, tx, claim, owner, seats)
	if err != nil {
		return err
	}
	switch {
	case !closing && !shown:
		return writeClaimStatus(ctx, tx, claim, row.contact, claimStatusDone, claimStatusOpen, map[string]any{"auto_reopened": true})
	case closing && shown:
		return s.keepWhenEvidenceHolds(ctx, tx, claim, row.contact, owner, hit.Meeting, seats)
	}
	return nil
}

// evidenceForOwner finds the meeting that shows the claim kept among the ones
// the owner may open. Nobody owning the promise means no evidence.
func (s *Store) evidenceForOwner(ctx context.Context, tx pgx.Tx, claim, owner ids.UUID, seats KeptSeats) (KeptEvidence, bool, error) {
	if owner.IsZero() {
		return KeptEvidence{}, false, nil
	}
	reader, err := seats.Reader(ctx, tx, owner)
	if err != nil {
		return KeptEvidence{}, false, err
	}
	visible, err := VisibleClaims(reader, tx, []ids.UUID{claim})
	if err != nil || !visible[claim] {
		return KeptEvidence{}, false, err
	}
	found, err := FindKeptEvidence(reader, tx, []ids.UUID{claim})
	hit, ok := found[claim]
	return hit, ok, err
}

// keepWhenEvidenceHolds closes the claim after share-locking the meeting and
// its guests and reading the evidence again. A decline committed since the
// first read is seen, and an uncommitted one makes the lock wait.
func (s *Store) keepWhenEvidenceHolds(ctx context.Context, tx pgx.Tx, claim, contactID, owner, meeting ids.UUID, seats KeptSeats) error {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity m WHERE m.id = $1 FOR SHARE`, meeting); err != nil {
		return fmt.Errorf("lock the meeting that shows the promise kept: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity_participant ap WHERE ap.activity_id = $1 FOR SHARE`, meeting); err != nil {
		return fmt.Errorf("lock the guests of the meeting that shows the promise kept: %w", err)
	}
	if _, still, err := s.evidenceForOwner(ctx, tx, claim, owner, seats); err != nil || !still {
		return err
	}
	return writeClaimStatus(ctx, tx, claim, contactID, claimStatusOpen, claimStatusDone, map[string]any{autoKeptKey: true})
}

// withAutoKept marks the done claims Margince closed. The evidence is read
// again under this reader's scope, so a meeting they may not open is not named.
func withAutoKept(ctx context.Context, tx pgx.Tx, claims []crmcontracts.ConversationClaim) error {
	var done []ids.UUID
	at := map[ids.UUID]int{}
	for i, claim := range claims {
		if claim.Kind == crmcontracts.ConversationClaimKindCommitmentOurs && claim.Status == crmcontracts.ConversationClaimStatusDone {
			done = append(done, ids.UUID(claim.Id))
			at[ids.UUID(claim.Id)] = i
		}
	}
	if len(done) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT c.id FROM conversation_claim c WHERE c.id = ANY($1)
		AND `+latestClaimImage+` ? '`+autoKeptKey+`'`, done)
	if err != nil {
		return fmt.Errorf("read which promises Margince closed: %w", err)
	}
	closed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("read which promises Margince closed: %w", err)
	}
	evidence, err := FindKeptEvidence(ctx, tx, closed)
	if err != nil {
		return err
	}
	for _, id := range closed {
		kept := &crmcontracts.ConversationClaimAutoKept{}
		if hit, ok := evidence[id]; ok {
			kept.Evidence = hit.contract()
		}
		claims[at[id]].AutoKept = kept
	}
	return nil
}

// ReopenAutoKeptClaim undoes Margince's close of a promise. Whoever may settle
// the claim by hand may undo it; a claim outside the caller's scope is absent.
// A claim a human closed is theirs to reopen through the settle path.
func (s *Store) ReopenAutoKeptClaim(ctx context.Context, claimID ids.UUID) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		contactID, current, err := lockWritableClaim(ctx, tx, claimID)
		if err != nil {
			return err
		}
		if current == claimStatusOpen {
			return nil
		}
		var auto bool
		if err := tx.QueryRow(ctx, `SELECT `+latestClaimImage+` ? '`+autoKeptKey+`' FROM conversation_claim c WHERE c.id = $1`,
			claimID).Scan(&auto); err != nil {
			return fmt.Errorf("read who closed the claim: %w", err)
		}
		if current != claimStatusDone || !auto {
			return fmt.Errorf("a human settled this claim, so only the settle path reopens it: %w", apperrors.ErrConflict)
		}
		return writeClaimStatus(ctx, tx, claimID, contactID, current, claimStatusOpen, nil)
	})
}

// ReopenConversationClaim implements (POST /claims/{id}/reopen).
func (h Handlers) ReopenConversationClaim(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.ReopenAutoKeptClaim(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
