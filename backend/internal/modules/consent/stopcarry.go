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
	// a suppression onto a subject the caller names. person:update is the same
	// grant the merge itself takes, and it is the honest bar: moving somebody's
	// stop onto another record is curation of both records, which is the act
	// the merge verb already maps to update.
	//
	// Deliberately not ratified into ungatedEntryPoints. An entry there has to
	// argue that no gate could apply; here one plainly does.
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// One statement, so the read of the predecessor's live rows and the write
	// of the survivor's cannot straddle a concurrent lift.
	//
	// The NOT EXISTS is the idempotence: it is evaluated per candidate row
	// against the survivor's live rows of the same kind, including rows this
	// same statement is inserting, because the subquery sees the table as of
	// the statement's snapshot plus its own effects.
	// RETURNING, so each carried stop can ship its own event. A consumer that
	// learned about the original suppression must learn that it now also
	// applies to the survivor, or it keeps mailing the record the merge just
	// made current.
	rows, err := tx.Query(ctx, `
		INSERT INTO communication_suppression
		  (person_id, lead_id, address, kind, source, decided_by_level,
		   captured_by, carried_from)
		SELECT $3, $4, live.address, live.kind, live.source, live.decided_by_level,
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
		            AND held.revoked_at IS NULL)
		RETURNING kind, decided_by_level`,
		zeroAsNull(from.PersonID.UUID), zeroAsNull(from.LeadID.UUID),
		zeroAsNull(to.PersonID.UUID), zeroAsNull(to.LeadID.UUID), by)
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
	entity, entityID := "person", to.PersonID.UUID
	if entityID.IsZero() {
		entity, entityID = "lead", to.LeadID.UUID
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

// zeroAsNull sends a zero uuid as SQL NULL, so the WHERE arms above can ask
// "is this side a person or a lead" with an IS NOT NULL rather than comparing
// against a sentinel value that is also a legal-looking uuid.
func zeroAsNull(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}
