// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/kernel/relstrength"

// visiblePartyRolesSQL are the header and meeting roles a row can show a
// contact in. A blind copy is not here: who was blind-copied is not disclosed
// by a row.
const visiblePartyRolesSQL = `('from', 'to', 'cc', 'organizer', 'attendee')`

// ContactIsPartySQL admits an activity, aliased `a`, the contact is a visible
// party to, by the rule WithContactRole reads. A link is not enough: a thread
// is linked to everybody it concerns. An unstated copy row is evidence only.
func ContactIsPartySQL(contact string) string {
	return `EXISTS (SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = a.id AND ap.role IN ` + visiblePartyRolesSQL + `
		   AND ap.response IS DISTINCT FROM 'declined'
		   AND ` + relstrength.ParticipantNamesSQL(contact, "ap") + `
		   AND NOT ` + unstatedContactSQL("ap") + `)`
}
