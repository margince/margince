// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sarRequestSections(pkg *SARPackage, subject ids.ContactID) []sarSection {
	args := []any{subject}
	where := storekit.SQLf("contact_id = $%[1]d OR subject_ref = $%[1]d::text", len(args))
	return []sarSection{
		{&pkg.RightsRequests, `SELECT kind, status, received_at, due_at, channel, receipt_reference, resolution FROM data_subject_request WHERE ` + where, args},
		{&pkg.RequestProofs, `SELECT p.request_number, p.requester_email, p.received_at,
   p.request_kind, p.identity_check, p.result, p.erased_counts, p.erased_uncounted,
   p.erased_counts_known, p.answered_at, p.keep_until
   FROM privacy_request_proof p JOIN data_subject_request d ON d.id = p.request_id
   WHERE ` + where, args},
	}
}
