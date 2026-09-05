// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The activity lifecycle beyond capture: update (completing a task is
// the everyday case), archive (visibility change — the 🟡 floor on the
// agent surface), and relink (moving a captured email onto the right
// deal WITHOUT touching its provenance — an association event, not a
// re-capture).

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type UpdateActivityInput struct {
	// Trail names what the audit trail calls this write; zero is an update.
	Trail      storekit.AuditTrail
	Subject    *string
	Body       *string
	OccurredAt *time.Time
	DueAt      *time.Time
	RemindAt   *time.Time
	AssigneeID *ids.UserID
	IsDone     *bool
	// MeetingStatus is how the meeting went, and it is meaningful only on a
	// meeting. The pairing is refused in the mapping against the kind the ROW
	// carries — a patch cannot change a kind, so the stored one is the only
	// honest thing to hold it against.
	MeetingStatus *string
	IfVersion     *int64
}

func (s *Store) UpdateActivity(ctx context.Context, id ids.ActivityID, in UpdateActivityInput) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The row lock makes the version compare and the coalesce update
		// below one race-free unit: without it two concurrent edits both
		// pass the compare and the loser silently overwrites the winner.
		held, err := lockActivityForWrite(ctx, tx, id.UUID)
		if err != nil {
			return err
		}
		// Reading the row is not the licence to change it: customer identity
		// is workspace-readable, so the write arm is what keeps a colleague's
		// correspondence theirs.
		if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
			return err
		}
		// A held row must reach the UPDATE below so its own CHECK trigger —
		// not this read — is what refuses the write.
		current, err := readActivityForWrite(ctx, tx, id, held)
		if err != nil {
			return err
		}
		// held skips this: activity_refuse_restricted_mutation below refuses
		// every write to a held row regardless of version, so it owes 423,
		// not a 409 inviting a retry the row can never accept.
		if !held && in.IfVersion != nil && current.Version != nil && int64(*current.Version) != *in.IfVersion {
			return apperrors.ErrVersionSkew
		}
		if err := renormalizeTranscriptPatch(current, &in); err != nil {
			return err
		}
		// The kind the ROW carries, not one the patch names — a patch cannot
		// change a kind. Without this a note could be given `held` and read back
		// afterwards as a meeting-shaped fact about something that was not one,
		// which is the pairing create already refuses; the database CHECK
		// constrains the vocabulary and not this.
		//
		// `held` skips it for the reason the version compare above skips it: a
		// row under retention hold owes 423 whatever else is wrong with the
		// request, and answering 422 first would invite the caller to fix the
		// field and try again against a row that will refuse them either way.
		if !held && in.MeetingStatus != nil && current.Kind != crmcontracts.ActivityKindMeeting {
			return &MeetingStatusKindError{Kind: string(current.Kind)}
		}
		if err := ensureAssigneeExists(ctx, tx, in.AssigneeID); err != nil {
			return err
		}
		// Every placeholder is derived from the argument slice rather than
		// typed. Nothing checks that a hand-written $N still names the value a
		// caller appends, and this statement's list has grown twice.
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		row := arg(id)
		// done_at travels WITH is_done (the activity_done_at CHECK):
		// completion stamps the moment, reopening clears it — so the flag is
		// named once and read three times.
		done := arg(in.IsDone)
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE activity SET
			  subject = coalesce($%[2]d, subject),
			  body = coalesce($%[3]d, body),
			  occurred_at = coalesce($%[4]d, occurred_at),
			  due_at = coalesce($%[5]d, due_at),
			  remind_at = coalesce($%[6]d, remind_at),
			  assignee_id = coalesce($%[7]d, assignee_id),
			  is_done = coalesce($%[8]d, is_done),
			  meeting_status = coalesce($%[9]d, meeting_status),
			  done_at = CASE
			    WHEN $%[8]d IS TRUE AND NOT is_done THEN now()
			    WHEN $%[8]d IS FALSE THEN NULL
			    ELSE done_at END
			WHERE id = $%[1]d`,
			row, arg(in.Subject), arg(in.Body), arg(in.OccurredAt), arg(in.DueAt),
			arg(in.RemindAt), arg(in.AssigneeID), done, arg(in.MeetingStatus)),
			args...); err != nil {
			return err
		}
		// Read back BEFORE auditing: done_at is stamped by the statement above
		// and a transcript body is renormalized on the way in, so the row is the
		// only place that says what this write actually stored.
		out, err = readActivity(ctx, tx, id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		// A transition is a CHANGE. A PATCH resending the status a meeting
		// already holds is somebody saving a form, and recording it would make
		// "booked twice" a countable event.
		if err := recordMeetingTransition(ctx, tx, meetingTransition{
			ActivityID:     id,
			Status:         changedMeetingStatus(current, out),
			ScheduledStart: &out.OccurredAt,
		}); err != nil {
			return err
		}
		before, after := storekit.ChangedColumns(activityColumnImage(current), activityColumnImage(out))
		auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "activity", id.UUID, before, after)
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: activityUpdatedChangedFields(in),
		})
	})
	return out, err
}

// renormalizeTranscriptPatch re-runs ADR-0058's normalizer on a body PATCH
// when the target row is transcript-marked. A transcript's normalized form
// is only ever produced on ingest (LogActivityInputFrom) — without this, a
// PATCH could leave a transcript-marked row holding un-normalized text (raw
// CRLFs, trailing whitespace), which is exactly the row the
// activity/transcript retention selector and any future line citation both
// assume is already canonical.
func renormalizeTranscriptPatch(current crmcontracts.Activity, in *UpdateActivityInput) error {
	if in.Body == nil || current.SourceSystem == nil || *current.SourceSystem != transcriptSourceSystem {
		return nil
	}
	normalized, err := normalizeTranscript(*in.Body)
	if err != nil {
		return err
	}
	in.Body = &normalized
	return nil
}

// ensureAssigneeExists checks a client-supplied user reference before it
// lands: the FK checks existence, RLS the tenancy. Nil means the patch
// doesn't touch the assignee, which is not this function's to gate.
func ensureAssigneeExists(ctx context.Context, tx pgx.Tx, assigneeID *ids.UserID) error {
	if assigneeID == nil {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1 AND status = 'active' AND archived_at IS NULL)`,
		*assigneeID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return apperrors.ErrNotFound
	}
	return nil
}

// RefuseArchiveActivity answers every authority refusal ArchiveActivity would
// answer with, and writes nothing.
//
// It exists so a confirm-first archive is refused BEFORE a human is asked
// rather than after they have answered: the probes below are the whole of
// what the archive requires of a caller, and asking them here is what keeps a
// staged approval from being spent on a call the store was always going to
// refuse. Deliberately NO version probe — a version that is right at staging
// can be wrong by the time the human answers, so the pin is the write's
// business and never a reason to refuse a staging.
//
// A held row's refusal is surfaced the same way ArchiveActivity's own is: by
// crossing activity_refuse_restricted_mutation, not by a second copy of what
// the trigger already says. The touch changes nothing (SET archived_at =
// archived_at), and the transaction never commits — the trigger refuses
// every write to a held row, so there is nothing left to roll back deliberately.
func (s *Store) RefuseArchiveActivity(ctx context.Context, id ids.ActivityID) error {
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		held, err := lockActivityForWrite(ctx, tx, id.UUID)
		if err != nil {
			return err
		}
		if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
			return err
		}
		if !held {
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE activity SET archived_at = archived_at WHERE id = $1`, id.UUID)
		return err
	})
}

// ArchiveActivity retires one activity, conditioned on ifVersion wherever the
// caller's authority named a version.
//
// The write rides storekit's guarded patch rather than a bare UPDATE, for the
// reason ApplyGuarded's own doc gives — *an unguarded update is not
// expressible* — which the archive verb was quietly the exception to. With a
// pin it is the optimistic CAS, so an archive a human released against version
// 4 lands on version 4 or answers skew; without one it takes the row lock, so
// it is never LESS guarded than the bare statement it replaces. The gone and
// already-archived cases keep answering ErrNotFound, which is the same
// existence-hiding answer as before.
func (s *Store) ArchiveActivity(ctx context.Context, id ids.ActivityID, ifVersion *int64) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		held, err := lockActivityForWrite(ctx, tx, id.UUID)
		if err != nil {
			return err
		}
		if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
			return err
		}
		p := storekit.NewPatch()
		p.Set("archived_at", nil, time.Now().UTC())
		// held drops the filter AND the pin: the filter lets the UPDATE reach
		// activity_refuse_restricted_mutation instead of a LiveOnly clause
		// hiding the row again, and the pin — a CAS by WHERE clause that never
		// reaches the trigger on a mismatch — would otherwise answer stale
		// version skew (409) instead of the reachable 423 on a row nothing
		// can write to regardless of version. Dropping it is safe: this
		// transaction already holds the row FOR UPDATE via
		// lockActivityForWrite, the guard an unpinned ApplyGuardedIn falls
		// back to.
		pin := ifVersion
		if held {
			pin = nil
		}
		if err := p.ApplyGuardedIn(ctx, tx, "activity", id.UUID, pin, activityArchivedFilter(held)); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "archive", "activity", id.UUID, nil, nil)
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityArchived{}); err != nil {
			return err
		}
		out, err = readActivity(ctx, tx, id, storekit.IncludeArchived)
		return err
	})
	return out, err
}

// relinkAdmittedRow is the transactional half both doors share: the target
// probe, then the guarded row write. The admission stays outside it so the
// single door can refuse a malformed request before it opens a transaction.
func relinkAdmittedRow(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in RelinkActivityInput, column string) (wrote, held bool, err error) {
	// The relink target is a client-supplied reference (H1).
	if err := auth.EnsureLinkTarget(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return false, false, err
	}
	return relinkActivityRow(ctx, tx, id, in, column)
}

// deleteVisibleLinksOfType drops the activity's links of one entity type and
// answers the person ids that delete actually displaced. Those ids come from
// the delete ITSELF. Inferring them instead — "whoever is a participant but no
// longer linked" — sweeps up participants that were never linked in the first
// place, and repoints conversations the correction never mentioned.
//
// Only the links this caller can SEE are replaced. An activity's own
// visibility derives from its links, so an unscoped delete lets someone who
// reached this activity through one link cut another — dropping a team's sight
// of a record by rewriting an association they were never shown.
//
// A link outside the caller's scope survives instead, and for `project` that
// used to leave a residual: at most one project link may exist, so the insert
// then hit the partial index and refused, and the difference between that
// refusal and a success told the caller a project link they could not see was
// there. One bit escaped, and hiding a link's existence while enforcing
// one-per-activity looked like the same question asked twice.
//
// It is closed, and closed on both halves rather than narrowed. A project
// carries no own/team arm (platform/auth tableclass.go) and no capture privacy
// either — migration 1787320003 narrowed its visibility CHECK to 'workspace',
// because nothing auto-creates a project and an owner-private one was a state
// no writer could reach. So no project link can be invisible to a caller
// holding the object grant: the delete reaches every one, and the move
// succeeds. The 23505 path below still stands for the caller who asks to
// associate rather than move.
func deleteVisibleLinksOfType(ctx context.Context, tx pgx.Tx, id ids.ActivityID, entityType, column string) ([]ids.UUID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, typePos := arg(id), arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, entityType, "t", arg)
	if err != nil {
		return nil, err
	}
	visible := "true"
	if scope != "" {
		visible = scope
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		DELETE FROM activity_link
		WHERE activity_id = $%d AND entity_type = $%d
		  AND EXISTS (SELECT 1 FROM %s t WHERE t.id = activity_link.%s AND %s)
		RETURNING person_id`,
		idPos, typePos, entityType, column, visible), args...)
	if err != nil {
		return nil, err
	}
	// Every id this delete actually removed. A link row of another
	// entity type returns NULL here and contributes nothing.
	displaced, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ids.UUID, error) {
		var pid *ids.UUID
		if err := r.Scan(&pid); err != nil {
			return ids.Nil, err
		}
		if pid == nil {
			return ids.Nil, nil
		}
		return *pid, nil
	})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(displaced, func(personID ids.UUID) bool { return personID == ids.Nil }), nil
}

// repointDisplacedParticipants moves the displaced contacts' participant rows
// onto the relink target. A relink to a PERSON is a human saying "this
// conversation was actually with someone else", so the participant row naming
// the old contact is now wrong (ACT-DDL-3). Repointing it keeps the
// participants and the links telling one story.
//
// The DISPLACED person carries the row scope too. The relink already gated the
// new target; without this the old one is rewritten sight unseen, so a caller
// could repoint a participant naming a contact they cannot read — including an
// owner-private captured one. The link delete scopes for the same reason; this
// is its participant twin.
//
// KNOWN GAP, stated rather than papered over: the graph consumer derives its
// affected (user, person) pairs from the participant rows, and by the time it
// runs they name the NEW contact — so the OLD edge is not recomputed and keeps
// counting an interaction that no longer points at it. The nightly rebuild
// clears it, which bounds the staleness to the same 24h the window counts
// already carry, but it is a bound and not a fix.
//
// The fix is the additive `relinked_from` reference ADR-0078 specifies on the
// activity.updated relink payload: the consumer needs the displaced id, and
// this module cannot recompute the edge itself because search is a sibling.
// That is a public-event contract change and belongs in its own slice.
func repointDisplacedParticipants(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) error {
	var pargs []any
	parg := func(v any) int { pargs = append(pargs, v); return len(pargs) }
	idPos, targetPos, displacedPos := parg(id), parg(target), parg(displaced)
	visible, err := auth.ScopeClauseFor(ctx, linkEntityPerson, "op", parg)
	if err != nil {
		return err
	}
	if visible == "" {
		// An unbounded caller narrows nothing.
		visible = "true"
	}
	// One merge, not a conditional rewrite. The repoint used to UPDATE each
	// displaced row to the target and skip when the target was already a
	// participant, which was wrong in both directions:
	//
	//   - the skip left the displaced row naming the OLD contact, so the
	//     activity's links said one thing and its participants another —
	//     exactly what the repoint exists to prevent — and that row kept
	//     feeding a relationship-strength signal for somebody the human had
	//     just said the conversation was not with;
	//   - with several displaced participants and no target row, every one of
	//     them qualified and each was rewritten to the target, colliding on
	//     uq_activity_participant.
	//
	// The uniqueness is per (activity, role, user, person, address), not per
	// person, so both the skip test and the collision are decided by the whole
	// tuple. Within each such group exactly one displaced row is promoted to
	// the target — and only when the target holds no row of that shape
	// already — and the rest are deleted. Either way the target is named once
	// and no displaced row survives.
	const nilUUID = `'00000000-0000-0000-0000-000000000000'::uuid`
	if _, err := tx.Exec(ctx, storekit.SQLf(`
		WITH scoped AS (
			SELECT ap.id, ap.role, ap.user_id, ap.address,
			       row_number() OVER (
			           PARTITION BY ap.role, coalesce(ap.user_id, `+nilUUID+`), coalesce(ap.address, '')
			           ORDER BY ap.id) AS rank
			  FROM activity_participant ap
			 WHERE ap.activity_id = $%d
			   -- Exactly the people the link delete removed, and no
			   -- others. A participant can name somebody who was never
			   -- linked at all, and inferring the displaced set from "no
			   -- longer linked" would rewrite them too.
			   AND ap.person_id = ANY($%d::uuid[])
			   AND ap.person_id <> $%d
			   AND EXISTS (SELECT 1 FROM person op WHERE op.id = ap.person_id AND (`+visible+`))
		), promoted AS (
			UPDATE activity_participant ap SET person_id = $%d
			  FROM scoped s
			 WHERE ap.id = s.id AND s.rank = 1
			   AND NOT EXISTS (
			       SELECT 1 FROM activity_participant other
			        WHERE other.activity_id = ap.activity_id
			          AND other.role = s.role
			          AND other.person_id = $%d
			          AND coalesce(other.user_id, `+nilUUID+`) = coalesce(s.user_id, `+nilUUID+`)
			          AND coalesce(other.address, '') = coalesce(s.address, ''))
			RETURNING ap.id
		)
		DELETE FROM activity_participant
		 WHERE id IN (SELECT id FROM scoped)
		   AND id NOT IN (SELECT id FROM promoted)`,
		idPos, displacedPos, targetPos, targetPos, targetPos), pargs...); err != nil {
		return err
	}
	return nil
}

// activityUpdatedChangedFields projects the patch's touched/untouched decisions
// onto activity.updated's BOUNDED changed_fields struct. body carries a presence
// flag, never the content — bodies can be large and are never echoed onto the
// wire.
//
// It reads the REQUEST while the audit image reads the row, and the two are not
// interchangeable: an event announces which fields a caller asked to change, an
// audit image records what the row then held.
func activityUpdatedChangedFields(in UpdateActivityInput) crmcontracts.PublicEventActivityChangedFields {
	var fields crmcontracts.PublicEventActivityChangedFields
	if in.Subject != nil {
		fields.Subject = in.Subject
	}
	if in.Body != nil {
		bodyTouched := true
		fields.Body = &bodyTouched
	}
	if in.OccurredAt != nil {
		fields.OccurredAt = in.OccurredAt
	}
	if in.DueAt != nil {
		fields.DueAt = in.DueAt
	}
	if in.RemindAt != nil {
		fields.RemindAt = in.RemindAt
	}
	if in.AssigneeID != nil {
		assignee := openapi_types.UUID(in.AssigneeID.UUID)
		fields.AssigneeId = &assignee
	}
	if in.IsDone != nil {
		fields.IsDone = in.IsDone
	}
	if in.MeetingStatus != nil {
		status := crmcontracts.PublicEventActivityChangedFieldsMeetingStatus(*in.MeetingStatus)
		fields.MeetingStatus = &status
	}
	return fields
}
