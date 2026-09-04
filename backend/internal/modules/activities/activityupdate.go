// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

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
		"subject":     derefOrNil(a.Subject),
		"body":        derefOrNil(a.Body),
		"occurred_at": a.OccurredAt,
		"due_at":      derefOrNil(a.DueAt),
		"remind_at":   derefOrNil(a.RemindAt),
		"assignee_id": derefOrNil(a.AssigneeId),
		"is_done":     derefOrNil(a.IsDone),
		"done_at":     derefOrNil(a.DoneAt),
		// Writable on the patch, so it belongs in the image: a column the update
		// can change and the diff cannot see leaves an audit row saying nothing
		// happened. "Who recorded that the meeting was a no-show" is exactly the
		// question the trail is read for.
		"meeting_status": derefOrNil(a.MeetingStatus),
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
