// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "fmt"

// Which attachments a record's Documents list holds, spelled once for the deal,
// contact and company readers.
//
// A captured mail's file hangs off the ACTIVITY, so every reader answers "files
// of the mails linked to this record" by walking the links as they are NOW. The
// company is never stamped on a mail file: a contact or company linked after
// capture would otherwise never pick the file up.

// Holder kinds a Documents list can be read for.
const (
	holderDeal    = linkEntityDeal
	holderContact = linkEntityContact
	holderCompany = linkEntityCompany
)

// mailFileMembership says an attachment is a live file of a live mail that
// satisfies linkedTo, a fragment over the activity aliased `a`, minus inline mail
// images.
func mailFileMembership(linkedTo string) string {
	return fmt.Sprintf(`(at.entity_type = '%s'
		AND NOT (COALESCE(at.content_type, '') LIKE 'image/%%' AND COALESCE(at.byte_size, 0) < %d)
		AND EXISTS (SELECT 1 FROM activity a WHERE a.id = at.entity_id AND a.archived_at IS NULL AND %s))`,
		linkEntityActivity, inlineImageCeiling, linkedTo)
}

// documentMembership says which attachments holder's Documents list holds,
// before any caller-bound visibility: the holder's own files and the files of
// every mail linked to it. pos is the bind position carrying the holder's id.
// Callers add visibleParentClause, which gates each row on its own parent.
func documentMembership(holder string, pos int) string {
	var own, linked string
	switch holder {
	case holderDeal, holderContact:
		own = fmt.Sprintf("at.entity_type = '%s' AND at.entity_id = $%d", holder, pos)
		linked = fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link l
			 WHERE l.activity_id = a.id AND l.entity_type = '%[1]s' AND l.%[1]s_id = $%[2]d)`, holder, pos)
	case holderCompany:
		// An upload keeps the company it was stamped with. A mail file is read
		// through the links and employment walk, without participants: a Cc'd
		// employee does not make the mail a document of their employer.
		own = fmt.Sprintf("at.entity_type <> '%s' AND at.company_id = $%d", linkEntityActivity, pos)
		linked = CompanyFiledActivityExists(pos)
	default:
		panic("activities: no document membership for holder " + holder)
	}
	return fmt.Sprintf("at.archived_at IS NULL AND NOT at.bytes_withheld AND ((%s) OR %s)",
		own, mailFileMembership(linked))
}
