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
	"strings"
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

// recheckWindow is how long after its meeting an automatic close is still
// compared with the meeting's answers.
const recheckWindow = 7 * 24 * time.Hour

// latestClaimImage is the audit image of the claim's newest change.
const latestClaimImage = `(SELECT a.after FROM audit_log a WHERE a.entity_type = 'contact'
	AND a.entity_id = c.contact_id AND a.after->>'claim_id' = c.id::text
	ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1)`

// reopenedByAHuman holds a claim someone took out of done. A person saying a
// promise is not kept outranks what the meeting shows.
var reopenedByAHuman = `EXISTS (SELECT 1 FROM audit_log h WHERE h.entity_type = 'contact'
	AND h.entity_id = c.contact_id AND h.after->>'claim_id' = c.id::text
	AND h.before->>'claim_status' = 'done' AND h.after->>'claim_status' = 'open' AND ` + storekit.AuditForAUser("h") + `)`

// KeptCandidate is a promise whose evidence the sweep checks, and the seat
// whose reading decides it.
type KeptCandidate struct {
	Claim  ids.UUID
	Owner  ids.UUID
	Closed bool
}

// KeptPromiseCandidatesTx lists the open promises to invite someone and the
// automatic closes still inside their recheck window, each with the seat
// responsible for it: the task's assignee, else who took it over, else the user
// who recorded it. A promise nobody is responsible for is left to its hint.
func (s *Store) KeptPromiseCandidatesTx(ctx context.Context, tx pgx.Tx) ([]KeptCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.status = 'done', t.assignee_id, c.taken_by_user_id, c.captured_by
		  FROM conversation_claim c
		  LEFT JOIN activity t ON t.id = c.task_activity_id AND t.archived_at IS NULL
		 WHERE c.kind = 'commitment_ours' AND c.archived_at IS NULL AND NOT c.needs_review
		   AND c.body ~* '\yinvit'
		   AND ((c.status = 'open' AND NOT `+reopenedByAHuman+`)
		     OR (c.status = 'done' AND `+latestClaimImage+` ? '`+autoKeptKey+`'
		         AND (`+latestClaimImage+`->'`+autoKeptKey+`'->>'meeting_at')::timestamptz > $1))
		 ORDER BY c.id`, s.clock().Add(-recheckWindow))
	if err != nil {
		return nil, fmt.Errorf("read the promises to check against meetings: %w", err)
	}
	defer rows.Close()
	var out []KeptCandidate
	for rows.Next() {
		var candidate KeptCandidate
		var assignee, taker *ids.UUID
		var capturedBy string
		if err := rows.Scan(&candidate.Claim, &candidate.Closed, &assignee, &taker, &capturedBy); err != nil {
			return nil, fmt.Errorf("scan a promise to check against meetings: %w", err)
		}
		if owner, ok := responsibleSeat(assignee, taker, capturedBy); ok {
			candidate.Owner = owner
			out = append(out, candidate)
		}
	}
	return out, rows.Err()
}

func responsibleSeat(assignee, taker *ids.UUID, capturedBy string) (ids.UUID, bool) {
	for _, seat := range []*ids.UUID{assignee, taker} {
		if seat != nil {
			return *seat, true
		}
	}
	if rest, ok := strings.CutPrefix(capturedBy, principal.HumanIDPrefix); ok {
		if seat, err := ids.Parse(rest); err == nil {
			return seat, true
		}
	}
	return ids.UUID{}, false
}

// ReconcileKeptPromisesTx brings the named promises in line with the meetings
// the owner may read. ctx is the system writer; reader carries the owner's
// scope and decides what counts as evidence. An automatic close whose evidence
// is gone is reopened first, so a promise that lost one meeting can close on
// another in the same pass.
func (s *Store) ReconcileKeptPromisesTx(ctx, reader context.Context, tx pgx.Tx, candidates []KeptCandidate) error {
	var named []ids.UUID
	for _, candidate := range candidates {
		named = append(named, candidate.Claim)
	}
	visible, err := VisibleClaims(reader, tx, named)
	if err != nil {
		return err
	}
	var readable []ids.UUID
	for _, claim := range named {
		if visible[claim] {
			readable = append(readable, claim)
		}
	}
	found, err := FindKeptEvidence(reader, tx, readable)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		hit, shown := found[candidate.Claim]
		switch {
		case candidate.Closed && !shown:
			err = autoMove(ctx, tx, candidate.Claim, claimStatusDone, claimStatusOpen, map[string]any{"auto_reopened": true})
		case !candidate.Closed && shown:
			err = autoMove(ctx, tx, candidate.Claim, claimStatusOpen, claimStatusDone, map[string]any{autoKeptKey: map[string]any{
				"meeting_id": hit.Meeting.String(), "meeting_at": hit.At, "guest_name": hit.Guest,
			}})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// autoMove moves one claim as the system, and only when it still stands where
// the pass read it. A person who settled or reopened it meanwhile wins.
func autoMove(ctx context.Context, tx pgx.Tx, claimID ids.UUID, from, to string, extra map[string]any) error {
	if _, err := storekit.LockRow(ctx, tx, "conversation_claim", claimID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	var contactID ids.UUID
	var current string
	var reopened bool
	if err := tx.QueryRow(ctx, `SELECT c.contact_id, c.status, `+reopenedByAHuman+` FROM conversation_claim c WHERE c.id = $1`,
		claimID).Scan(&contactID, &current, &reopened); err != nil {
		return fmt.Errorf("read the claim to move: %w", err)
	}
	if current != from || (to == claimStatusDone && reopened) {
		return nil
	}
	return writeClaimStatus(ctx, tx, claimID, contactID, from, to, extra)
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
// A claim a person closed is theirs to reopen through the settle path.
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
			return fmt.Errorf("a person settled this claim, so only the settle path reopens it: %w", apperrors.ErrConflict)
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
