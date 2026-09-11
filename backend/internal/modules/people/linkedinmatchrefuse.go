// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Refusing a LinkedIn suggestion, which is the answer that ENDS one.
//
// Split from linkedinmatchapply.go because it is the opposite verb over the
// same row and it has its own authority story: an apply writes to a PERSON and
// takes person:update for it, where a refusal writes only the connection's own
// terminal state and takes the read grant its sibling writer takes. Keeping
// them in one file made that difference read as an inconsistency rather than as
// the two different things being written.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordLinkedInMatchRefused writes the terminal state a refused suggestion has
// always had a value for and never a writer.
//
// The migration that created the column defines `rejected`; nothing in the tree
// wrote it. Rejecting a proposal runs no effect — the approvals engine
// dispatches its effect table on APPROVE only — so a refused connection stayed
// `suggested` and non-tombstoned for ever, and nothing reading the row could
// tell a finished suggestion from a live one.
//
// The refusal itself was never lost: StageUnlessDeclined reads the declined
// offers and will not re-propose. What was lost is the cost. The sweep
// enumerates every connection in (unmatched, suggested), so a refused one paid
// an RBAC resolve, a whole-network match, a pending read and a staging
// transaction on every hourly pass and every company event, for a
// guaranteed-empty result — a permanent per-pass charge that grows with the
// size of the imported network, for a state that is supposed to be the cheap
// one.
//
// Written where the refusal is OBSERVED rather than where it is made: the
// stager already learns it, because StageUnlessDeclined answers "not staged"
// for exactly this reason. That keeps the whole change on this side of the
// seam — no reject-side effect table, which would be a new concept in the
// approvals engine for one caller — and it is self-healing: a connection
// refused before this shipped is marked the first time a sweep reaches it.
//
// Idempotent by predicate. A second pass finds the row already rejected, the
// UPDATE matches nothing, and no audit row is written for a decision nobody
// made twice.
func (s *Store) RecordLinkedInMatchRefused(ctx context.Context, connectionID ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return apperrors.ErrPermissionDenied
	}
	// person:read, the grant its SIBLING WRITER of this column takes.
	// MatchLinkedInConnections sets match_status to `suggested` under the read
	// grant, because what these rows are is one member's own imported network
	// and the authority over them is owning them. person:update is what
	// ApplyLinkedInMatch takes, and it takes it because it writes to a PERSON —
	// copying that here would refuse the whole staging pass for a ghost owner
	// holding read-only person grants, which is most of them.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// No person: the sweep is answering "this identity was declined", which
		// is what StageUnlessDeclined told it, and that is a fact about the
		// connection rather than about one pair. The decline effect below has
		// the staged pair and binds on it.
		return refuseLinkedInMatchInTx(ctx, tx, connectionID, actor.UserID, ids.Nil)
	})
}

// RecordLinkedInMatchRefusedTx marks the connection terminal inside a
// transaction the CALLER owns, for the reject effect.
//
// It exists so a rejection and the mark commit together: run afterwards, a
// failure would leave the card rejected and the ghost still suggested, which is
// the disagreement between the two records this whole write exists to close.
//
// Bound to the PAIR the proposal named, on all three axes, exactly as
// ApplyLinkedInMatchTx binds its own — a refusal is an answer about one
// suggestion, and between the staging and this write the matcher may have moved
// the row to a different contact. Refusing the row it points at NOW would
// discard a suggestion nobody was asked about.
func RecordLinkedInMatchRefusedTx(ctx context.Context, tx pgx.Tx, connectionID, ownerID, personID ids.UUID) error {
	return refuseLinkedInMatchInTx(ctx, tx, connectionID, ownerID, personID)
}

// refuseLinkedInMatchInTx is the write both callers land on.
//
// A ZERO personID means the caller is refusing the SUGGESTION whatever pair it
// names, which is the sweep's question; a set one binds the pair, which is the
// decision's. The difference is in what each caller knows, not in what a
// refusal means, so it is a parameter rather than two statements.
func refuseLinkedInMatchInTx(ctx context.Context, tx pgx.Tx, connectionID, ownerID, personID ids.UUID) error {
	if ownerID == ids.Nil {
		return fmt.Errorf("people: refusing a LinkedIn match names no owner: %w", apperrors.ErrPermissionDenied)
	}
	var wasStatus string
	var wasPerson *ids.UUID
	// matched_person_id goes with the status. The row no longer proposes
	// anybody, and leaving the id behind would keep a pair claim on a row whose
	// answer was no — matchRankOrder's rejected slot and the `<> 'rejected'`
	// predicate both read this row as carrying no live suggestion.
	err := tx.QueryRow(ctx, `
			UPDATE linkedin_connection c
			   SET match_status = $2, matched_person_id = NULL, updated_at = now()
			  FROM linkedin_connection was
			 WHERE c.id = $1 AND was.id = c.id AND c.tombstoned_at IS NULL
			   AND c.owner_user_id = $3
			   AND c.match_status = $4
			   AND ($5::uuid IS NULL OR c.matched_person_id = $5)
			 RETURNING was.match_status, was.matched_person_id`,
		connectionID, matchRejected, ownerID, matchSuggested, optionalPerson(personID)).Scan(&wasStatus, &wasPerson)
	if errors.Is(err, pgx.ErrNoRows) {
		// Every way this matches nothing is a no-op and none is wrong.
		//
		// SUGGESTED and nothing else, which is the narrow predicate and the
		// one that matters: the refusal was observed against a snapshot, and
		// between that read and this write the row may have been confirmed
		// by the exact-name matcher or reset to unmatched by a re-import.
		// A refusal is only ever about the suggestion it answered, so a
		// stale observation must leave a newer state alone rather than
		// overwrite a link somebody now has.
		//
		// The owner clause is the same authority the read had: one member
		// marking another's connection is not a refusal they made.
		//
		// Already rejected, tombstoned or gone are the ordinary cases — the
		// sweep re-reaches a refused row on every pass until the
		// enumeration stops covering it, which is the point of it being
		// cheap to answer.
		return nil
	}
	if err != nil {
		return fmt.Errorf("people: recording a refused LinkedIn match: %w", err)
	}
	images := matchImages(wasStatus, wasPerson, matchRejected, nil)
	_, err = storekit.Audit(ctx, tx, "update", "linkedin_connection", connectionID, images.before, images.after)
	return err
}
