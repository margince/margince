// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Carrying a retiring subject's stops onto the record that survives them.
//
// people owns the merge and calls this inside its transaction (people's
// StopCarrier seam, wired in compose). It owns person_consent and carries that
// itself; communication_suppression is ours, and until this existed a merge
// moved the grants and left the stops behind — pointing at an id no send
// evaluates any more, so marketing resumed against somebody who had refused it
// with nothing in the audit saying a stop was dropped.
//
// COPY, NEVER MOVE. The predecessor's row stays exactly as written:
//
//   - It is evidence. "This person objected on 2 September" is a fact about
//     that record, and rewriting its subject to point at the survivor would
//     make the history say the objection was made about somebody else.
//   - The predecessor may be un-merged, or read in an export, long after.
//   - An erasure that later reaches the predecessor must find its own rows.
//
// So the survivor gets a NEW row carrying the same kind, the same authority
// and the same words, and the two are linked by carried_from so a reader can
// see which act produced which.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// CarryStopsTx implements people.StopCarrier.
//
// IDEMPOTENT ON KIND. A survivor who already holds a live stop of the same
// kind keeps their own — theirs is at least as recent and may carry a
// different authority, and two live rows of one kind would mean the second
// lift silently re-enables mail the first was still refusing.
//
// THE AUTHORITY TRAVELS. A subject-level objection stays subject-level on the
// survivor, so no seat can lift what the subject asked for merely by merging
// the record first. Carrying it as the merging rep's own level would be a
// laundering path: merge, then lift.
func (s *Store) CarryStopsTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	// GATED HERE TOO, not merely at the merge that calls it.
	//
	// This is exported and takes a transaction it does not own, so anything
	// holding a consent store can call it directly — and its effect is to write
	// a suppression onto a subject the caller names.
	//
	// THE GRANT FOLLOWS THE SUBJECT BEING WRITTEN, which is the survivor. A
	// person merge already holds person:update, and a lead merge or promotion
	// holds lead:update — grants are independent, so requiring person:update of
	// every caller refused the lead paths outright, including when there was
	// nothing to carry.
	object := entityPerson
	if to.PersonID.IsZero() {
		object = entityLead
	}
	if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// LOCKED FIRST, because carry and lift are otherwise unserialized: a lift
	// committing between this read and the write could revoke the very row
	// that made the carry skip, leaving the survivor unstopped. lift.go takes
	// the same advisory lock on the subject, so the two now queue.
	if err := lockOneSubjectsSuppressions(ctx, tx, to); err != nil {
		return err
	}

	// DISTINCT ON collapses several live source rows of one kind to the
	// strongest, because a subquery in an INSERT ... SELECT does NOT see the
	// rows that statement is inserting — Postgres evaluates it against the
	// statement's snapshot. Without this, a subject holding two live
	// subject_requests carried two copies onto the survivor.
	//
	// The NOT EXISTS compares AUTHORITY, not merely kind. A survivor holding a
	// weaker row of the same kind must not block a stronger one: a legacy
	// user-level subject_request would otherwise keep a subject-level objection
	// out, and an admin could then lift the weaker row — the laundering path
	// carrying the authority was meant to close.
	// RETURNING, so each carried stop can ship its own event. A consumer that
	// learned about the original suppression must learn that it now also
	// applies to the survivor, or it keeps mailing the record the merge just
	// made current.
	rows, err := tx.Query(ctx, `
		INSERT INTO communication_suppression
		  (person_id, lead_id, address, kind, source, decided_by_level,
		   captured_by, carried_from)
		SELECT DISTINCT ON (live.kind)
		       $3, $4, live.address, live.kind, live.source, live.decided_by_level,
		       $5, live.id
		  FROM communication_suppression live
		 WHERE (($1::uuid IS NOT NULL AND live.person_id = $1)
		     OR ($2::uuid IS NOT NULL AND live.lead_id = $2))
		   AND live.revoked_at IS NULL
		   AND NOT EXISTS (
		         SELECT 1 FROM communication_suppression held
		          WHERE (($3::uuid IS NOT NULL AND held.person_id = $3)
		              OR ($4::uuid IS NOT NULL AND held.lead_id = $4))
		            AND held.kind = live.kind
		            AND held.revoked_at IS NULL
		            AND coalesce(array_position($6::text[], held.decided_by_level), array_length($6::text[], 1) + 1)
		                >= coalesce(array_position($6::text[], live.decided_by_level), array_length($6::text[], 1) + 1))
		 ORDER BY live.kind, coalesce(array_position($6::text[], live.decided_by_level), array_length($6::text[], 1) + 1) DESC, live.recorded_at DESC
		RETURNING kind, decided_by_level`,
		zeroAsNull(from.PersonID.UUID), zeroAsNull(from.LeadID.UUID),
		zeroAsNull(to.PersonID.UUID), zeroAsNull(to.LeadID.UUID), by, authorityLadder())
	if err != nil {
		return fmt.Errorf("consent: carrying the subject's stops onto the surviving record: %w", err)
	}
	type carried struct {
		kind  string
		level string
	}
	var moved []carried
	for rows.Next() {
		var c carried
		if err := rows.Scan(&c.kind, &c.level); err != nil {
			rows.Close()
			return fmt.Errorf("consent: reading the carried stops: %w", err)
		}
		moved = append(moved, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("consent: carrying the subject's stops onto the surviving record: %w", err)
	}
	if len(moved) == 0 {
		// Nothing to carry is the ordinary case — most people have no stop —
		// and it is not worth an audit row.
		return nil
	}
	// Audited against the SURVIVOR, which is the record whose sending
	// behaviour just changed. A reader asking "why is this person suppressed"
	// finds the merge that brought it.
	entity, entityID := entityPerson, to.PersonID.UUID
	if entityID.IsZero() {
		entity, entityID = entityLead, to.LeadID.UUID
	}
	// AuditEvent, not Audit: an `update` audit demands a before-image, and
	// there is no prior state to record. The survivor did not have a stop that
	// changed — a stop was added to them, which is a write with an after and
	// no before.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", entity, entityID,
		map[string]any{"stops_carried": len(moved)})
	if err != nil {
		return err
	}
	// One event per carried stop, the same consent.suppressed a directly
	// recorded one ships — a consumer cannot tell the two apart, and should
	// not: the survivor is suppressed either way.
	for _, c := range moved {
		if err := storekit.EmitEvent(ctx, tx, auditID, entityID,
			suppressionRecordedPayload(c.kind, commsauthz.AuthorityLevel(c.level))); err != nil {
			return err
		}
	}
	return nil
}

// authorityLadder renders commsauthz's own rank order for SQL, weakest first,
// so the comparison in the carry is the one CanOverrule makes and not a second
// copy of it. An unknown level is absent from the ladder and the query ranks it
// ABOVE every known one, matching rank()'s default: a level this build does not
// understand must not be overruled by one it does.
func authorityLadder() []string {
	levels := commsauthz.LevelsWeakestFirst()
	out := make([]string, 0, len(levels))
	for _, l := range levels {
		out = append(out, string(l))
	}
	return out
}

// lockOneSubjectsSuppressions takes the SAME advisory lock lift.go takes, on
// the same key, so a carry and a lift touching one subject queue instead of
// racing.
//
// Without it the two interleave in a way that loses a stop: carry reads the
// survivor's live rows, a lift revokes the row carry just saw and skipped, and
// carry commits having decided there was nothing to add. The survivor ends up
// with no stop at all, which is the failure this whole file exists to prevent.
func lockOneSubjectsSuppressions(ctx context.Context, tx pgx.Tx, subject commsauthz.StopSubject) error {
	key := subject.PersonID.String()
	if subject.PersonID.IsZero() {
		key = subject.LeadID.String()
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("consent: serialising stop writes for this subject: %w", err)
	}
	return nil
}

// zeroAsNull sends a zero uuid as SQL NULL, so the WHERE arms above can ask
// "is this side a person or a lead" with an IS NOT NULL rather than comparing
// against a sentinel value that is also a legal-looking uuid.
func zeroAsNull(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}
