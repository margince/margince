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
		var err error
		out, err = updateActivityInTx(ctx, tx, id, in)
		return err
	})
	return out, err
}

// updateActivityInTx is UpdateActivity's transactional body, shared with the
// capture replay in activity.go.
//
// Shared rather than copied because a second spelling of "a meeting moved"
// would be one that could stop recording the transition, stop emitting the
// bounded delta, or stop taking the write lock — and the only symptom would be
// a lead ladder that never re-read a meeting somebody had cancelled.
//
// The RBAC check is NOT here. It sits at UpdateActivity, where a caller is
// asking to patch a record; the replay is not asking for that authority, it is
// finishing the capture it already had — and the row-scope arm below still
// applies to both.
func updateActivityInTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, in UpdateActivityInput,
) (crmcontracts.Activity, error) {
	current, err := admitActivityPatch(ctx, tx, id, &in)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
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
		return crmcontracts.Activity{}, err
	}
	// Read back BEFORE auditing: done_at is stamped by the statement above
	// and a transcript body is renormalized on the way in, so the row is the
	// only place that says what this write actually stored.
	out, err = readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// A transition is a CHANGE. A PATCH resending the status a meeting
	// already holds is somebody saving a form, and recording it would make
	// "booked twice" a countable event.
	if err := recordMeetingTransition(ctx, tx, meetingTransition{
		ActivityID:     id,
		Status:         changedMeetingStatus(current, out),
		ScheduledStart: &out.OccurredAt,
	}); err != nil {
		return crmcontracts.Activity{}, err
	}
	before, after := storekit.ChangedColumns(activityColumnImage(current), activityColumnImage(out))
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "activity", id.UUID, before, after)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: activityUpdatedChangedFields(in),
	}); err != nil {
		return crmcontracts.Activity{}, err
	}
	return out, nil
}

// admitActivityPatch takes the write lock and answers whether this patch may
// land on this row, returning the row as it stood before it.
//
// Every refusal here is about the TARGET rather than the values: who may write
// the row, whether the version the caller compared against is still current,
// and whether the field being set means anything on the kind the row carries. A
// patch that gets past all of them is one the UPDATE can apply without asking
// anything further.
//
// It takes `in` by pointer for one reason: renormalizeTranscriptPatch rewrites
// the body on the way in, and a copy would leave the caller applying the text
// the request carried rather than the canonical form.
func admitActivityPatch(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, in *UpdateActivityInput,
) (crmcontracts.Activity, error) {
	// The row lock makes the version compare and the coalesce update
	// below one race-free unit: without it two concurrent edits both
	// pass the compare and the loser silently overwrites the winner.
	held, err := lockActivityForWrite(ctx, tx, id.UUID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// Reading the row is not the licence to change it: customer identity
	// is workspace-readable, so the write arm is what keeps a colleague's
	// correspondence theirs.
	if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, !held); err != nil {
		return crmcontracts.Activity{}, err
	}
	// A held row must reach the UPDATE afterwards so its own CHECK trigger —
	// not this read — is what refuses the write.
	current, err := readActivityForWrite(ctx, tx, id, held)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// held skips this: activity_refuse_restricted_mutation refuses every write
	// to a held row regardless of version, so it owes 423, not a 409 inviting a
	// retry the row can never accept.
	if !held && in.IfVersion != nil && current.Version != nil && int64(*current.Version) != *in.IfVersion {
		return crmcontracts.Activity{}, apperrors.ErrVersionSkew
	}
	if err := renormalizeTranscriptPatch(current, in); err != nil {
		return crmcontracts.Activity{}, err
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
		return crmcontracts.Activity{}, &MeetingStatusKindError{Kind: string(current.Kind)}
	}
	if err := ensureAssigneeCanHoldWork(ctx, tx, in.AssigneeID); err != nil {
		return crmcontracts.Activity{}, err
	}
	return current, nil
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
