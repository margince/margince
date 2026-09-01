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
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// cohortRepairBatch bounds ONE pass, in messages of each kind.
//
// The repair runs on the verdict's own user-facing transaction as well as on the
// consumer, and a contact whose address matches years of correspondence would
// otherwise write every link and publish every event in one go — a transaction
// nobody sized, holding locks while it does it. The bound makes the cost of a
// pass knowable instead.
//
// A cohort larger than this is not lost: what the pass leaves behind still
// matches its own selection, so the next person event repairs the next batch,
// and the nightly reconcile is what guarantees the tail is reached without
// waiting for one. The number is deliberately generous — an ordinary contact's
// whole history is far below it, so the batching is invisible in every case but
// the pathological one.
const cohortRepairBatch = 500

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
	batch := cohortRepairBatch
	linked, err := linkCapturedCohort(ctx, tx, canonical)
	if err != nil {
		return CohortPromotion{}, err
	}
	promoted, err := promoteParticipantsToPerson(ctx, tx, nil, canonical, &batch)
	if err != nil {
		return CohortPromotion{}, err
	}
	if len(linked) == 0 && len(promoted) == 0 {
		// Nothing moved. A repair that found nothing is not an event in this
		// person's history, and auditing every replayed person.updated would
		// bury the passes that did change something.
		return CohortPromotion{}, nil
	}
	// A repair that moved rows is a change to which record a message belongs
	// to, and that is exactly what the trail is for: mail appears on a contact's
	// timeline without anybody clicking, and the audit row is the only place
	// that says why. AuditEvent rather than Audit because there is no
	// before-image to judge — the subject is the person whose cohort moved, not
	// any one of the messages.
	auditID, err := storekit.AuditEvent(ctx, tx, "update", entityPerson, canonical.UUID,
		map[string]any{"cohort_linked": len(linked), "cohort_promoted": len(promoted)})
	if err != nil {
		return CohortPromotion{}, fmt.Errorf("people: auditing a cohort repair: %w", err)
	}
	// One activity.updated per message that moved. The interaction graph folds
	// its edges from participant rows on an activity event, so a repair that
	// stayed silent would leave "who knows this contact" answering from the
	// state before the mail arrived on their record — correct only until the
	// next unrelated write happened to refold it.
	for _, activity := range changedActivities(linked, promoted) {
		if err := storekit.EmitEvent(ctx, tx, auditID, activity, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{
				Relinked: &crmcontracts.PublicEventActivityRelinkedRef{
					EntityType: entityPerson,
					EntityId:   openapi_types.UUID(canonical.UUID),
				},
			},
		}); err != nil {
			return CohortPromotion{}, fmt.Errorf("people: publishing a cohort repair: %w", err)
		}
	}
	return CohortPromotion{Linked: int64(len(linked)), Promoted: int64(len(promoted))}, nil
}

// changedActivities folds the two halves of a pass into the messages to publish,
// dropping a repeat. An activity commonly appears in both — the same message
// gains its link and has its participant row named in one go — and it needs one
// event either way, because the graph refolds the whole activity rather than one
// row of it.
//
// A participant-only promotion counts. The interaction graph folds its edges
// from participant rows, so a message whose link already existed and whose
// participant row just learned the person is precisely a message whose edges
// are now wrong, and staying silent about it leaves "who knows this contact"
// answering from before.
func changedActivities(linked, promoted []ids.UUID) []ids.UUID {
	seen := make(map[ids.UUID]struct{}, len(linked)+len(promoted))
	out := make([]ids.UUID, 0, len(linked)+len(promoted))
	for _, group := range [][]ids.UUID{linked, promoted} {
		for _, id := range group {
			if _, already := seen[id]; already {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
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
func linkCapturedCohort(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
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
		 ORDER BY a.occurred_at DESC, a.id
		 LIMIT $2
		ON CONFLICT DO NOTHING
		RETURNING activity_id`, personID, cohortRepairBatch)
	if err != nil {
		return nil, fmt.Errorf("people: linking a person's captured cohort: %w", err)
	}
	defer rows.Close()
	var linked []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("people: linking a person's captured cohort: %w", err)
		}
		linked = append(linked, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: linking a person's captured cohort: %w", err)
	}
	return linked, nil
}
