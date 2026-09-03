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
	// After the promotion, never before it: this reads the person-resolved
	// attendee rows the promotion above has just written.
	meetings, err := linkAttendedMeetings(ctx, tx, canonical)
	if err != nil {
		return CohortPromotion{}, err
	}
	// And after those, for the same ordering reason: the invitation that named
	// this attendee in full is on a meeting they are only now resolved on. This
	// is the second of the two namings — the meeting synced before they were a
	// contact — and without it a person minted from a bare invitation address
	// keeps the local part of their own email as their name forever.
	if err := fillPersonNameFromAttendance(ctx, tx, canonical); err != nil {
		return CohortPromotion{}, err
	}
	// A meeting this pass has just FILED is no longer the link-less record the
	// capture limiter held. Re-deriving says so: until this ran, a meeting
	// captured before its attendee was a contact stayed participants-only
	// forever, so the meeting on a colleague's page was invisible to everyone
	// but the people on the invitation while the invitation emails beside it
	// were workspace-readable. Only the meetings, and only the ones this call
	// linked — the derivation itself decides whether the hold is still true
	// (activities.noRecordHoldStands).
	if err := s.rederiveAudiences(ctx, tx, meetings); err != nil {
		return CohortPromotion{}, err
	}
	linked = append(linked, meetings...)
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

// PeopleOwedACohortRepair lists contacts whose captured mail is not all on their
// record yet, up to limit, ordered by id so a tick takes a stable slice.
//
// The two arms are the two halves the repair fixes, and each one asks its
// question the same way the write does — including the guards. That symmetry is
// what makes the scan terminate: a row the write refuses (a second address of a
// party already recorded, say) must not be offered again on every pass, or the
// sweep never drains and the job looks stuck rather than done.
//
// Only LIVE, unmerged contacts are offered, and that single filter is what makes
// a merge safe here. A merged-away record still owns participant rows — the
// merge repoints them now, but a database written before it does not — and
// repairing under that id would write links onto a page no read returns, then
// find the same rows again on the next tick. Resolving the redirect inside the
// arms as well would be a second guard for the one case this already covers,
// and two guards that each pass alone are one guard with a spare.
func (s *Store) PeopleOwedACohortRepair(ctx context.Context, limit int) ([]ids.PersonID, error) {
	var out []ids.PersonID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH owed AS (
				SELECT pe.person_id
				  FROM activity_participant ap
				  JOIN person_email pe ON lower(pe.email) = ap.address AND pe.archived_at IS NULL
				 WHERE ap.person_id IS NULL AND ap.user_id IS NULL AND ap.address IS NOT NULL
				   AND NOT EXISTS (
				       SELECT 1 FROM activity_participant other
				        WHERE other.activity_id = ap.activity_id
				          AND other.role = ap.role
				          AND other.person_id = pe.person_id)
				UNION
				SELECT pe.person_id
				  FROM activity a
				  JOIN person_email pe ON pe.email = a.counterparty_email AND pe.archived_at IS NULL
				 WHERE a.captured_by LIKE 'connector:%' AND a.restricted_at IS NULL
				   AND NOT EXISTS (
				       SELECT 1 FROM activity_link l
				        WHERE l.activity_id = a.id AND l.person_id IS NOT NULL)
				UNION
				-- Meetings this contact attended but is not filed under. A
				-- separate arm because the one above finds work by
				-- counterparty_email, and a meeting has none: attendance is a
				-- list, so the only thing naming the attendee is a participant
				-- row. Without this arm a person owed nothing BUT meeting links
				-- is never offered, and the sweep reports a drained backlog
				-- while every synced meeting stays off their page.
				SELECT coalesce(att.merged_into_id, att.id)
				  FROM activity_participant ap
				  JOIN activity a ON a.id = ap.activity_id
				  JOIN person att ON att.id = ap.person_id
				 WHERE ap.person_id IS NOT NULL
				   AND a.kind = 'meeting'
				   AND a.captured_by LIKE 'connector:%' AND a.restricted_at IS NULL
				   AND a.archived_at IS NULL
				   AND NOT EXISTS (
				       SELECT 1 FROM activity_link l
				        WHERE l.activity_id = a.id
				          AND l.person_id = coalesce(att.merged_into_id, att.id))
				   AND (SELECT count(*) FROM activity_link cap
				         WHERE cap.activity_id = a.id) < $2
			)
			SELECT owed.person_id FROM owed
			  JOIN person live ON live.id = owed.person_id
			 WHERE live.archived_at IS NULL AND live.merged_into_id IS NULL
			 ORDER BY owed.person_id
			 LIMIT $1`, limit, maxMeetingLinksPerActivity)
		if err != nil {
			return fmt.Errorf("people: listing the contacts owed a cohort repair: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.PersonID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("people: listing the contacts owed a cohort repair: %w", err)
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RepairPersonCohort is PromotePersonCohortTx on its own transaction, for the
// sweep that owns no other write.
func (s *Store) RepairPersonCohort(ctx context.Context, personID ids.PersonID) (CohortPromotion, error) {
	var done CohortPromotion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		done, err = s.PromotePersonCohortTx(ctx, tx, personID)
		return err
	})
	if err != nil {
		return CohortPromotion{}, err
	}
	return done, nil
}

// DomainsOwedTheirPeople lists companies whose domain has live contacts with no
// employer yet, up to limit.
//
// They accumulate while nobody has a company for the domain: capture creates
// each person and deliberately leaves the employer undecided, so by the time the
// company is recorded its whole roster is attached to nothing. The account then
// shows ONE contact — whichever sender writes next and earns an edge from their
// own ensure — and the health card blames the entire relationship on them.
//
// It is a SWEEP rather than a write on the create path, and that is the point.
// Attaching a person to a company is a write about the PERSON, and the human
// recording a company holds no authority over contacts they may not even see —
// a rep scoped to their own records would otherwise plant employment for a
// colleague's private contact by typing in a company name. The sweep runs as the
// system, which is the only actor that honestly may touch all of them.
func (s *Store) DomainsOwedTheirPeople(ctx context.Context, limit int) ([]DomainBacklog, error) {
	var out []DomainBacklog
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT od.organization_id, od.domain
			  FROM organization_domain od
			  JOIN organization o ON o.id = od.organization_id
			  JOIN person_email pe
			    ON pe.archived_at IS NULL
			    -- The SAME two arms the plant matches on, subdomain included. A
			    -- selector narrower than the write leaves a backlog nothing ever
			    -- offers; one wider offers work the write will not take, and the
			    -- domain is returned on every tick for ever.
			   AND (split_part(pe.email, '@', 2) = od.domain
			        OR right(split_part(pe.email, '@', 2), length(od.domain) + 1) = '.' || od.domain)
			  JOIN person p ON p.id = pe.person_id
			 WHERE od.archived_at IS NULL
			   AND o.archived_at IS NULL AND o.merged_into_id IS NULL
			   AND p.archived_at IS NULL AND p.merged_into_id IS NULL
			   AND NOT EXISTS (
			       SELECT 1 FROM relationship r
			        WHERE r.person_id = p.id AND `+CurrentPrimarySlotSQL("r")+`)
			   -- And not a person the index will refuse anyway. uq_rel_employment
			   -- admits ONE live employment per (person, organization), so
			   -- somebody already holding a non-primary edge to this company is a
			   -- row the plant's ON CONFLICT silently drops — offering them again
			   -- is a domain that can never drain.
			   AND NOT EXISTS (
			       SELECT 1 FROM relationship held
			        WHERE held.person_id = p.id AND held.organization_id = od.organization_id
			          AND `+LiveEmploymentSlotSQL("held")+`)
			 ORDER BY od.organization_id, od.domain
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("people: listing the domains owed their people: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var owed DomainBacklog
			if err := rows.Scan(&owed.OrganizationID, &owed.Domain); err != nil {
				return fmt.Errorf("people: listing the domains owed their people: %w", err)
			}
			out = append(out, owed)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DomainBacklog names one company domain whose people are not attached yet.
type DomainBacklog struct {
	OrganizationID ids.OrganizationID
	Domain         string
}

// AttachDomainBacklog gives the live people on one company domain their
// employment edge, and answers how many it planted.
//
// The same plant the domain-triage verdict runs — it never reassigns anybody a
// human already placed, and it takes its own row locks.
func (s *Store) AttachDomainBacklog(ctx context.Context, owed DomainBacklog) (int, error) {
	var planted int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The company was live when the selector read it, in an earlier
		// transaction. Re-check it here under its own lock: an archive or a
		// merge in between would otherwise attach a domain's people to a record
		// no read returns, which is the failure this whole sweep exists to
		// repair rather than create.
		live, err := organizationIsLive(ctx, tx, owed.OrganizationID)
		if err != nil {
			return err
		}
		if !live {
			return nil
		}
		planted, err = plantDomainEmployment(ctx, tx, owed.Domain, owed.OrganizationID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return planted, nil
}
