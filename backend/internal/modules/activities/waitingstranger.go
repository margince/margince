// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// strangerMailSQL is the three facts capture stored about a stranger's mail.
// NULL reads as false. Capture judged Bcc for the mailbox the mail came
// through, so the lane also checks that this reader is in neither To nor Cc.
const strangerMailSQL = `coalesce(a.sender_was_stranger, false),
	       (coalesce(a.bcc_only, false) AND NOT EXISTS (
	          SELECT 1 FROM activity_participant openly
	           WHERE openly.activity_id = a.id AND openly.role IN ('to', 'cc')
	             AND lower(openly.address) = ANY(%[16]s::text[]))),
	       coalesce(a.foreign_link, false)`
