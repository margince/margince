// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// UpdateActivityInput is a sparse activity edit. A nil field stays as stored.
type UpdateActivityInput struct {
	DueIntent          *crmcontracts.IntentTime
	RemindIntent       *crmcontracts.IntentTime
	MeetingIntent      *crmcontracts.IntentTime
	DealFiling         *crmcontracts.ActivityDealFiling
	recordedAt         *time.Time
	sourceActivityID   *ids.UUID
	calendarSettlement bool
	// recordZone dates a stored instant made a day; the store sets it (WithRecordZone).
	recordZone RecordZone
	// Trail names what the audit trail calls this write; zero is an update.
	Trail      storekit.AuditTrail
	Subject    *string
	Body       *string
	OccurredAt *time.Time
	// OccurredPrecision nil with OccurredAt set means an instant: a new time is
	// a known time. A day sent alone keeps the stored date, moved to its anchor.
	OccurredPrecision *string
	DurationSeconds   *int
	DueAt             *time.Time
	// DueHasTime says whether DueAt names a time of day. Sent only with DueAt;
	// a DueAt without it is a day (admitPatchTime).
	DueHasTime *bool
	// TaskType retypes a task; refused on another kind.
	TaskType   *crmcontracts.TaskType
	RemindAt   *time.Time
	AssigneeID *ids.UserID
	IsDone     *bool
	// Clear names the fields sent as an explicit JSON null, which a nil pointer
	// cannot tell from a field not mentioned.
	Clear []string
	// A patch cannot change kind, so MeetingStatus is checked against the stored kind.
	MeetingStatus *string
	// CallOutcome is how a call ended; refused on another kind, like
	// MeetingStatus, against the kind the row carries.
	CallOutcome *string
	IfVersion   *int64
	// CustomFields are the patch's cf_ values; a null clears one. customColumns
	// are the active fields they are checked and written against.
	CustomFields  map[string]any
	customColumns []fieldcatalog.Column
	// doneAtAnyVersion lets a caller's Done land on a task already done at any
	// version. A machine completion leaves it off: its skew says it did not flip the task.
	doneAtAnyVersion bool
}

// UpdateActivity keeps an authorized edit and its audit in one transaction.
func (s *Store) UpdateActivity(ctx context.Context, id ids.ActivityID, in UpdateActivityInput) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.Activity{}, err
	}
	at := s.now()
	in.recordedAt, in.recordZone = &at, s.recordZone
	if err := s.readPatchColumns(ctx, &in); err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err := s.txRetryingLockCycles(ctx, pgx.ReadCommitted, func(tx pgx.Tx) error {
		var err error
		out, err = updateActivityInTx(ctx, tx, id, in)
		if err == nil {
			out, err = s.fileUpdatedActivity(ctx, tx, out, in)
		}
		return err
	})
	if err != nil {
		return out, err
	}
	return out, s.attachCustomValues(ctx, &out)
}

// activityColumnImage renders the columns a PATCH can move, for one side of an
// audit diff.
//
// It is built from the ROW rather than from the request, and the after side is
// read back after the write, because two of these columns are the database's to
// decide: done_at travels with is_done under the activity_done_at CHECK, and a
// transcript's body is renormalized on the way in. An image assembled from the
// incoming patch would record a body the row does not hold and a done_at that
// was never stamped.
//
// The body is carried whole. It is the field a reader most often wants back,
// and "the body changed" without the text is a history entry nobody can act on.
//
// Reading content off this row is sound because every caller reaches it past
// EnsureActivityWritable, whose first act is EnsureActivityContentVisibleLive:
// a principal who may edit the activity may read its content, so the row here is
// never a withheld one with its subject and body nulled out.
func activityColumnImage(a crmcontracts.Activity) map[string]any {
	return map[string]any{
		fieldSubject:           derefOrNil(a.Subject),
		fieldBody:              derefOrNil(a.Body),
		auditFieldOccurredAt:   a.OccurredAt,
		fieldOccurredPrecision: string(a.OccurredPrecision),
		fieldDurationSeconds:   derefOrNil(a.DurationSeconds),
		"due_at":               derefOrNil(a.DueAt),
		"due_intent":           derefOrNil(a.DueIntent),
		"remind_intent":        derefOrNil(a.RemindIntent),
		fieldMeetingIntent:     derefOrNil(a.MeetingIntent),
		"source_activity_id":   derefOrNil(a.SourceActivityId),
		fieldDueHasTime:        derefOrNil(a.DueHasTime),
		"remind_at":            derefOrNil(a.RemindAt),
		"assignee_id":          derefOrNil(a.AssigneeId),
		auditFieldIsDone:       derefOrNil(a.IsDone),
		"done_at":              derefOrNil(a.DoneAt),
		// Writable on the patch, so it belongs in the image: a column the update
		// can change and the diff cannot see leaves an audit row saying nothing
		// happened. "Who recorded that the meeting was a no-show" is exactly the
		// question the trail is read for.
		"meeting_status": derefOrNil(a.MeetingStatus),
		// How a call ended: writable on the patch, so the diff must see it.
		fieldCallOutcome: derefOrNil(a.CallOutcome),
		fieldTaskType:    derefOrNil(a.TaskType),
	}
}

// derefOrNil renders an optional column as the value it holds, or as the
// absence itself. A *T left as a pointer would reach the audit image as a JSON
// object address's contents on one side and a nil on the other, so the diff
// would compare a value against a pointer and call every column changed.
//
//craft:ignore naked-any one audit image column, whichever SQL type the field carries
func derefOrNil[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
}
