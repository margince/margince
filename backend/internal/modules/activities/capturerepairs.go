// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// CaptureLabelsDataTask registers the bounded rename of derived labels.
func CaptureLabelsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794701217_capture_remainder_is_other", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1794701217_capture_remainder_is_other"),
		Requires: []string{"core/1794701217"},
		Steps: []datatask.Step{{
			Name: "capture_labels", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "Derived capture labels are unaudited, as in activities.SetCaptureLabelAndReply."},
			Estimate: CountActivitiesToInspect, Batch: RenameCaptureLabelsTx,
		}},
	}
}

// MeetingNotesDataTask connects stored notes through the store's meeting writer.
func MeetingNotesDataTask(store *Store) datatask.Task {
	return datatask.Task{
		ID: "core/1794701220_automatic_notes_record_their_meeting", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1794701220_automatic_notes_record_their_meeting"),
		Requires: []string{"core/1794412054"},
		Steps: []datatask.Step{{
			Name: "record_meetings", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: CountActivitiesToInspect, Batch: store.RepairMeetingNotesTx,
		}},
	}
}
