// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "fmt"

func priorDispositionExpression(address, reach string) string {
	return fmt.Sprintf(`CASE
		         WHEN EXISTS (
		           SELECT 1 FROM contact_email pe JOIN contact p ON p.id = pe.contact_id
		            WHERE pe.email = %[1]s AND p.archived_at IS NULL
		              AND pe.from_correspondence) THEN 'real'
		         ELSE coalesce((
		           -- A real-status row whose KIND names no human is not an
		           -- instruction to create one. role_mailbox and
		           -- company_sender resolve to real because the mail is
		           -- genuine correspondence that must stay visible — but there
		           -- is nobody to record, and reading the status alone would
		           -- create the very contact the verdict declined to create,
		           -- on the sender's second message.
		           SELECT CASE
		                    WHEN status = 'real' AND kind IS NOT NULL AND kind <> 'contact'
		                      THEN 'known_noncontact'
		                    ELSE status
		                  END
		             FROM capture_pending_counterparty
		            WHERE email = %[1]s
		              AND status IN ('real', 'noise', 'rejected', 'suppressed')
		              -- A noise answer settles only the mail it can still reach.
		              -- Past that, this message is new evidence and gets its own
		              -- question: otherwise one forged message would silently
		              -- bar an address forever, and later mail would be neither
		              -- judged nor hidden -- the worst of both. The other
		              -- answers do not expire, because real and a human decline
		              -- are decisions about the SENDER, not about one message.
		              AND (status <> 'noise'
		                   OR resolved_at IS NULL
		                   OR resolved_at >= now() - make_interval(secs => %[2]s))
		            ORDER BY resolved_at DESC NULLS LAST
		            LIMIT 1), '')
		       END`, address, reach)
}
