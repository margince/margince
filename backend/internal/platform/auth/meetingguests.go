// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// guestRowsSQL is the guest rows, alias guest, that may admit a reader of
// activity act. Each is judged against the activity it was captured on, alias
// guest_of. A transcript that records a meeting is read by the meeting's guests
// under the meeting's own standing rule, not by a copy of them. Every other
// activity is read by its own guests.
func guestRowsSQL(act string) string {
	return fmt.Sprintf(`activity_participant guest JOIN activity guest_of ON guest_of.id = guest.activity_id
	 WHERE guest.activity_id = CASE WHEN %[1]s.source_system = '%[2]s' AND %[1]s.source_activity_id IS NOT NULL
	                               THEN %[1]s.source_activity_id ELSE %[1]s.id END
	   AND (guest_of.id = %[1]s.id OR guest_of.kind = 'meeting')`, act, provenance.MeetingTranscriptSource)
}
