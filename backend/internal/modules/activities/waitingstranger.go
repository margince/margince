// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// strangerMailSQL is the three facts capture stored about a stranger's mail.
// NULL reads as false. Bcc is judged per seat on capture_import, so only the
// reader's own import says they got it by Bcc (%[9]d is the reader).
// A To or Cc row naming one of the reader's addresses overrides it.
const strangerMailSQL = `coalesce(a.sender_was_stranger, false),
	       (EXISTS (
	          SELECT 1 FROM capture_import mine_import
	           WHERE mine_import.activity_id = a.id AND mine_import.user_id = $%[9]d
	             AND mine_import.bcc_only)
	        AND NOT EXISTS (
	          SELECT 1 FROM activity_participant openly
	           WHERE openly.activity_id = a.id AND openly.role IN ('to', 'cc')
	             AND lower(openly.address) = ANY(%[16]s::text[]))),
	       coalesce(a.foreign_link, false)`
