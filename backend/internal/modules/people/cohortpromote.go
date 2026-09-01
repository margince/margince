// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Repairing a person's whole captured cohort, not just the message that
// happened to create them.
//
// linkActivityToPerson settles ONE activity — the message the ensure ran for.
// That is the right scope at capture time and the wrong one everywhere else,
// because the record can arrive after the mail: a backfill walks newest-first
// and creates the person from a message ten minutes into the run, a human types
// a contact in, a verdict resolves a question that was open while the sender
// kept writing. Every message captured before that moment keeps an address-only
// participant row and no link, and no reader of activity_link finds it again.
//
// So the promotion is also spelled ONCE at cohort scope, and the two spellings
// share their statements rather than resembling each other. The invariant this
// holds: the final state of activity_link and activity_participant does not
// depend on the ORDER in which the activity and the person arrived.
//
// Ownership: activity_link and activity_participant belong to activities, and
// people writes both here under the waivers that already ratify
// linkActivityToPerson and namePersonAmongParticipants. The reason generalizes
// from those — people is the only module that can settle the merge redirect and
// read person_email, which is what the cohort is DEFINED by, and activities can
// do neither without importing a sibling.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CohortPromotion is what one repair pass did, so a caller counts honestly
// rather than reporting the work it attempted.
type CohortPromotion struct {
	// Linked is activities that gained a person link they did not have.
	Linked int64
	// Promoted is address-only participant rows that now name the person.
	Promoted int64
}

// PromotePersonCohortTx repairs every captured activity this person's live
// addresses reach: the missing links, and the participant rows that named them
// only by address.
//
// The person is settled against a merge FIRST, for the reason
// linkActivityToPerson settles it: no reader of activity_link walks
// merged_into_id, so a link written to a retired id leaves the message on a
// record nobody opens.
func (s *Store) PromotePersonCohortTx(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID,
) (CohortPromotion, error) {
	var canonical ids.PersonID
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(merged_into_id, id) FROM person WHERE id = $1 FOR UPDATE`,
		personID).Scan(&canonical); err != nil {
		return CohortPromotion{}, fmt.Errorf("people: resolving the person a cohort belongs to: %w", err)
	}
	linked, err := linkCapturedCohort(ctx, tx, canonical)
	if err != nil {
		return CohortPromotion{}, err
	}
	promoted, err := promoteParticipantsToPerson(ctx, tx, nil, canonical)
	if err != nil {
		return CohortPromotion{}, err
	}
	return CohortPromotion{Linked: linked, Promoted: promoted}, nil
}

// linkCapturedCohort attaches every captured message from any of the person's
// live addresses to them.
//
// Three bounds, each load-bearing:
//
// Connector mail only, because a hand-logged activity carries the links a human
// chose and an inference from an address must not add to them.
//
// A restricted record is excluded rather than inherited: its counterparty_email
// is cleared so it cannot match anyway, but linking one to a live person would
// put it back in a reader's reach through that person's timeline.
//
// Only mail linked to NOBODY. A message already attached to a person belongs to
// that person's record, and a cohort inference about an address must not
// relabel it. This is deliberately wider than linkActivityToPerson's guard,
// which refuses only the identical link: that write acts on a decision about one
// named message, and this one acts on an address.
//
// No kind gate. The per-message ensure links whatever kind carried a
// counterparty, so keying on the counterparty reproduces exactly what a
// person-first ordering would have produced — and asks nothing of a kind that
// never carries one.
func linkCapturedCohort(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (int64, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		SELECT a.id, 'person', $1
		  FROM activity a
		 WHERE a.captured_by LIKE 'connector:%'
		   AND a.restricted_at IS NULL
		   AND a.counterparty_email IN (
		       SELECT lower(pe.email) FROM person_email pe
		        WHERE pe.person_id = $1 AND pe.archived_at IS NULL)
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_link l
		        WHERE l.activity_id = a.id AND l.person_id IS NOT NULL)
		ON CONFLICT DO NOTHING`, personID)
	if err != nil {
		return 0, fmt.Errorf("people: linking a person's captured cohort: %w", err)
	}
	return tag.RowsAffected(), nil
}
