// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/kernel/values"

// calendarResourceParticipantSQL also recognizes legacy contacts created from resource addresses.
func calendarResourceParticipantSQL(contactScope string) string {
	return `(COALESCE(` + values.CalendarResourceDomainSQL("ap.address_domain_folded") + `,false)
 OR (ap.address IS NULL AND EXISTS (
 SELECT 1 FROM contact cs WHERE cs.id=ap.contact_id AND cs.archived_at IS NULL AND ` + contactScope + `
 AND EXISTS (SELECT 1 FROM contact_email resource_address WHERE resource_address.contact_id=cs.id
     AND resource_address.archived_at IS NULL AND ` + values.CalendarResourceDomainSQL("resource_address.email_domain_folded") + `)
 AND NOT EXISTS (SELECT 1 FROM contact_email other_address WHERE other_address.contact_id=cs.id
     AND other_address.archived_at IS NULL AND NOT COALESCE(` + values.CalendarResourceDomainSQL("other_address.email_domain_folded") + `,false)))))`
}
