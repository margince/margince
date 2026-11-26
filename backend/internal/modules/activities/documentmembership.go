// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Which attachments a record's Documents list holds, spelled once for the deal,
// contact, company and project readers.
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
	holderProject = linkEntityProject
)

// mailFileIDs is the ids of the live, listed files of live mails that satisfy
// linkedTo. linkedTo is a fragment over the activity aliased `a`.
func mailFileIDs(linkedTo string) string {
	return fmt.Sprintf(`SELECT mf.id FROM activity a
		  JOIN attachment mf ON mf.entity_type = '%s' AND mf.entity_id = a.id AND mf.archived_at IS NULL
		 WHERE a.archived_at IS NULL AND %s AND %s`,
		linkEntityActivity, linkedTo, storekit.ListedMailFileSQL("mf"))
}

// documentMembership says which attachments holder's Documents list holds:
// its own files and the files of every mail linked to it. pos binds the
// holder's id. Callers add visibleParentClause, which gates each row.
//
// The arms are a `UNION ALL` of ids, so the plan starts from the holder.
// An `OR` reads every mail file of the installation. No file is in both arms.
//
// A link row has no bytes and is admitted by its address. The deal room's
// predicate leaves that arm out, so a link never reaches a buyer's download.
func documentMembership(holder string, pos int) string {
	var own, linked string
	switch holder {
	case holderDeal:
		own = fmt.Sprintf("own.entity_type = '%s' AND own.entity_id = $%d", holder, pos)
		linked = storekit.MessageOnDealSQL("a.id", fmt.Sprintf("$%d", pos))
	case holderContact, holderProject:
		own = fmt.Sprintf("own.entity_type = '%s' AND own.entity_id = $%d", holder, pos)
		linked = fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link l
			 WHERE l.activity_id = a.id AND l.entity_type = '%[1]s' AND l.%[1]s_id = $%[2]d)`, holder, pos)
	case holderCompany:
		// A deal's upload follows the deal: its company is read as it is now,
		// so a deal moved to another company takes its files along. The deals
		// are read first, as an array, so the plan starts from the account.
		//
		// A mail file is read through the links and employment walk without
		// participants, so a Cc'd employee does not file it with their employer.
		own = fmt.Sprintf(`own.entity_type = '%[1]s' AND own.entity_id = $%[3]d AND own.archived_at IS NULL
			UNION ALL SELECT own.id FROM attachment own
			 WHERE own.entity_type = '%[2]s'
			   AND own.entity_id = ANY (ARRAY(SELECT filed.id FROM deal filed WHERE filed.company_id = $%[3]d))`,
			linkEntityCompany, linkEntityDeal, pos)
		linked = CompanyFiledActivityExists(pos)
	default:
		//craft:ignore panic-in-domain a holder outside the four constants is a coding error, never request data
		panic("activities: no document membership for holder " + holder)
	}
	return fmt.Sprintf(`at.archived_at IS NULL AND (NOT at.bytes_withheld OR at.url IS NOT NULL) AND at.id IN (
		SELECT own.id FROM attachment own WHERE %s AND own.archived_at IS NULL
		UNION ALL %s)`, own, mailFileIDs(linked))
}

// arrivedWithMessage says an attachment is a file that came with its message. A
// row with an address is a pointer, not a file it carried. The paperclip count
// and the mail's file list both leave it out through this one fragment.
func arrivedWithMessage(alias string) string {
	return alias + ".url IS NULL"
}
