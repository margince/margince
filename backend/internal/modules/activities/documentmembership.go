// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "fmt"

// Which attachments a record's Documents list holds, spelled once for the deal,
// contact and company readers.
//
// A captured mail's file hangs off the activity. Every reader answers "files
// of the mails linked to this record" by walking the links as they are now. The
// company is never stamped on a mail file: a contact or company linked after
// capture would otherwise never pick the file up.

// Holder kinds a Documents list can be read for.
const (
	holderDeal    = linkEntityDeal
	holderContact = linkEntityContact
	holderCompany = linkEntityCompany
)

// mailFileIDs is the ids of the live files of live mails that satisfy linkedTo,
// a fragment over the activity aliased `a`, minus inline mail images.
func mailFileIDs(linkedTo string) string {
	return fmt.Sprintf(`SELECT mf.id FROM activity a
		  JOIN attachment mf ON mf.entity_type = '%s' AND mf.entity_id = a.id AND mf.archived_at IS NULL
		 WHERE a.archived_at IS NULL AND %s
		   AND NOT (COALESCE(mf.content_type, '') LIKE 'image/%%' AND COALESCE(mf.byte_size, 0) < %d)`,
		linkEntityActivity, linkedTo, inlineImageCeiling)
}

// documentMembership says which attachments holder's Documents list holds:
// its own files and the files of every mail linked to it. pos binds the
// holder's id. Callers add visibleParentClause, which gates each row.
//
// The arms are a `UNION ALL` of ids, so the plan starts from the holder.
// An `OR` reads every mail file of the installation. No file is in both arms.
func documentMembership(holder string, pos int) string {
	var own, linked string
	switch holder {
	case holderDeal, holderContact:
		own = fmt.Sprintf("own.entity_type = '%s' AND own.entity_id = $%d", holder, pos)
		linked = fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link l
			 WHERE l.activity_id = a.id AND l.entity_type = '%[1]s' AND l.%[1]s_id = $%[2]d)`, holder, pos)
	case holderCompany:
		// An upload keeps the company it was stamped with. A mail file is read
		// through the links and employment walk without participants. A Cc'd
		// employee does not make the mail a document of their employer.
		own = fmt.Sprintf("own.company_id = $%d AND own.entity_type <> '%s'", pos, linkEntityActivity)
		linked = CompanyFiledActivityExists(pos)
	default:
		//craft:ignore panic-in-domain a holder outside the three constants is a coding error, never request data
		panic("activities: no document membership for holder " + holder)
	}
	return fmt.Sprintf(`at.archived_at IS NULL AND (NOT at.bytes_withheld OR at.category = '%s') AND at.id IN (
		SELECT own.id FROM attachment own WHERE %s AND own.archived_at IS NULL
		UNION ALL %s)`, categoryLink, own, mailFileIDs(linked))
}

// arrivedWithMessage says an attachment is a file that came with its message. A
// link row is a pointer the message mentioned, not a file it carried. The
// paperclip count and the mail's file list both leave it out through this one
// fragment.
func arrivedWithMessage(alias string) string {
	return alias + ".category <> '" + categoryLink + "'"
}
