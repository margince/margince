// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// sarEvidenceSections gather the proof rows written about the subject's
// messages and leads: which door attested a message, and which notice a lead
// was sent.
func sarEvidenceSections(pkg *SARPackage, leads []ids.UUID) []sarSection {
	return []sarSection{
		// The identity each disclosed message answers to, and which door
		// attested it. A limited message keeps its identity back with its body.
		{&pkg.MessageIdentities, `SELECT ai.activity_id, ai.identity_kind, ai.identity_key, ai.attested_by, ai.created_at
		   FROM activity_identity ai
		   JOIN activity a ON a.id = ai.activity_id
		   JOIN activity_link l ON l.activity_id = a.id
		   WHERE l.contact_id = $1 AND ` + sarContentDisclosed, nil},
		// The Art. 14 notice a lead twin was sent: which wording, in which
		// language, and whether it went. approval_id and delivery_id are
		// withheld: they point at rows with their own sections.
		{&pkg.LeadPrivacyNotices, `SELECT template_key, template_version, locale, state, failure,
		          queued_at, failed_at, created_at
		   FROM lead_privacy_notice
		   WHERE lead_id = ANY($1)`, []any{leads}},
	}
}
